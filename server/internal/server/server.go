package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/metacubex/quic-go"

	"shieldlink-server/internal/auth"
	"shieldlink-server/internal/config"
	"shieldlink-server/internal/log"
	"shieldlink-server/internal/relay"
	"shieldlink-server/internal/transport"
)

type Server struct {
	cfg  *config.Config
	auth *auth.MultiAuthenticator
}

func New(cfg *config.Config) *Server {
	routes := make([]auth.Route, len(cfg.Routes))
	for i, r := range cfg.Routes {
		routes[i] = auth.Route{UUID: r.UUID, Forward: r.Forward}
	}
	return &Server{
		cfg:  cfg,
		auth: auth.NewMultiAuthenticator(routes),
	}
}

// Auth returns the multi-authenticator for admin API access.
func (s *Server) Auth() *auth.MultiAuthenticator {
	return s.auth
}

func (s *Server) Run() error {
	switch s.cfg.Protocol {
	case "udp":
		return s.runQUIC()
	case "both":
		// Start QUIC in background, TLS in foreground
		go func() {
			if err := s.runQUIC(); err != nil {
				log.L.Error("QUIC listener error", "err", err)
			}
		}()
		return s.runTLS()
	default:
		return s.runTLS()
	}
}

// ==================== TCP/TLS mode ====================

func (s *Server) runTLS() error {
	ln, err := transport.NewTLSListener(
		s.cfg.Listen,
		s.cfg.TLS.CertPath, s.cfg.TLS.KeyPath,
		s.cfg.TLS.AutoCert,
		s.cfg.MPTCP,
	)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	log.L.Info("server started (TCP/TLS)",
		"listen", s.cfg.Listen,
		"forward", s.cfg.Forward,
		"ip_passthrough", s.cfg.IPPassthrough,
		"mptcp", s.cfg.MPTCP,
	)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.L.Error("accept error", "err", err)
			continue
		}
		go s.handleTCPConn(conn)
	}
}

func (s *Server) handleTCPConn(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.L.Debug("new TCP connection", "remote", remoteAddr)

	buf := make([]byte, 16384)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		log.L.Debug("read error", "remote", remoteAddr, "err", err)
		return
	}
	conn.SetReadDeadline(time.Time{})

	header, route, err := s.auth.Verify(buf[:n])
	if err != nil {
		log.L.Warn("auth failed", "remote", remoteAddr, "err", err)
		return
	}

	log.L.Info("TCP authenticated",
		"remote", remoteAddr,
		"session", fmt.Sprintf("%x", header.SessionID),
		"forward", route.Forward,
		"initial_data_len", len(header.InitialData),
	)

	target, err := net.DialTimeout("tcp", route.Forward, 10*time.Second)
	if err != nil {
		log.L.Error("dial forward failed", "forward", route.Forward, "err", err)
		return
	}
	defer target.Close()

	if s.cfg.IPPassthrough && (header.Flags&auth.FlagIPPassthrough != 0) {
		ppHeader := BuildProxyProtocolV2(conn.RemoteAddr(), target.LocalAddr())
		if ppHeader != nil {
			if _, err := target.Write(ppHeader); err != nil {
				log.L.Error("write proxy protocol failed", "err", err)
				return
			}
		}
	}

	upload, download := relay.TCPRelay(conn, target, header.InitialData)
	log.L.Info("TCP closed", "remote", remoteAddr, "upload", upload, "download", download)
}

// ==================== UDP/QUIC mode ====================

func (s *Server) runQUIC() error {
	ln, err := transport.NewQUICListener(
		s.cfg.Listen,
		s.cfg.TLS.CertPath, s.cfg.TLS.KeyPath,
		s.cfg.TLS.AutoCert,
	)
	if err != nil {
		return fmt.Errorf("quic listen: %w", err)
	}

	log.L.Info("server started (UDP/QUIC)",
		"listen", s.cfg.Listen,
		"forward", s.cfg.Forward,
		"ip_passthrough", s.cfg.IPPassthrough,
	)

	for {
		conn, err := ln.Accept(context.Background())
		if err != nil {
			log.L.Error("quic accept error", "err", err)
			continue
		}
		go s.handleQUICConn(conn)
	}
}

func (s *Server) handleQUICConn(qconn *quic.Conn) {
	remoteAddr := qconn.RemoteAddr().String()
	log.L.Debug("new QUIC connection", "remote", remoteAddr)

	// Accept first stream (control stream for auth)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := qconn.AcceptStream(ctx)
	if err != nil {
		log.L.Debug("accept stream error", "remote", remoteAddr, "err", err)
		qconn.CloseWithError(1, "no stream")
		return
	}

	// Read auth header from stream
	buf := make([]byte, 16384)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		log.L.Debug("read stream error", "remote", remoteAddr, "err", err)
		qconn.CloseWithError(2, "read error")
		return
	}

	header, route, err := s.auth.Verify(buf[:n])
	if err != nil {
		log.L.Warn("QUIC auth failed", "remote", remoteAddr, "err", err)
		qconn.CloseWithError(3, "auth failed")
		return
	}

	log.L.Info("QUIC authenticated",
		"remote", remoteAddr,
		"session", fmt.Sprintf("%x", header.SessionID),
		"forward", route.Forward,
	)

	// After auth, the QUIC stream becomes a bidirectional tunnel (like TCP)
	target, err := net.DialTimeout("tcp", route.Forward, 10*time.Second)
	if err != nil {
		log.L.Error("dial forward failed", "forward", route.Forward, "err", err)
		qconn.CloseWithError(4, "forward error")
		return
	}
	defer target.Close()

	if s.cfg.IPPassthrough && (header.Flags&auth.FlagIPPassthrough != 0) {
		ppHeader := BuildProxyProtocolV2(qconn.RemoteAddr(), target.LocalAddr())
		if ppHeader != nil {
			target.Write(ppHeader)
		}
	}

	// Send any initial data
	if len(header.InitialData) > 0 {
		target.Write(header.InitialData)
	}

	// Bidirectional relay: QUIC stream <-> TCP target
	upload, download := relay.StreamRelay(stream, target)
	log.L.Info("QUIC closed", "remote", remoteAddr, "upload", upload, "download", download)
}
