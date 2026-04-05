package shieldlink

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/metacubex/mihomo/log"
)

// StartEmbeddedListener starts a local TCP listener that transparently tunnels
// all incoming connections through ShieldLink to remote servers.
// Returns the local address (host, port) that the proxy node should connect to.
func StartEmbeddedListener(cfg *ClientConfig) (host string, port int, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", 0, err
	}

	addr := ln.Addr().(*net.TCPAddr)
	pool := NewPool(cfg)

	log.Infoln("[ShieldLink] embedded listener on 127.0.0.1:%d -> %d server(s), uuid=%s...%s",
		addr.Port, len(pool.endpoints), cfg.UUID[:4], cfg.UUID[len(cfg.UUID)-4:])

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleLocalConn(conn, pool, cfg)
		}
	}()

	return "127.0.0.1", addr.Port, nil
}

func handleLocalConn(local net.Conn, pool *Pool, cfg *ClientConfig) {
	defer local.Close()

	if cfg.Aggregate {
		handleAggregateConn(local, pool, cfg)
		return
	}

	tunnel, err := pool.DialWithFailover()
	if err != nil {
		log.Warnln("[ShieldLink] tunnel dial failed: %v", err)
		return
	}
	defer tunnel.Close()

	log.Debugln("[ShieldLink] relay %s <-> tunnel", local.RemoteAddr())
	relay(local, tunnel)
}

func handleAggregateConn(local net.Conn, pool *Pool, cfg *ClientConfig) {
	// Connect to ALL alive servers for upload (split path)
	var serverConns []net.Conn
	for _, ep := range pool.endpoints {
		if !ep.alive.Load() {
			continue
		}
		conn, err := pool.DialTunnel(ep)
		if err != nil {
			log.Warnln("[ShieldLink] aggregate: skip %s: %v", ep.Address, err)
			continue
		}
		serverConns = append(serverConns, conn)
	}
	if len(serverConns) == 0 {
		log.Warnln("[ShieldLink] aggregate: no alive servers")
		return
	}

	sessionID := NewSessionID()
	var aggSession [8]byte
	copy(aggSession[:], sessionID[:])

	// Connect download channel
	var mergeConn net.Conn
	if cfg.MergeAddress != "" {
		// Legacy: connect to merge server directly
		var err error
		mergeConn, err = net.DialTimeout("tcp", cfg.MergeAddress, 10*time.Second)
		if err != nil {
			log.Warnln("[ShieldLink] aggregate: merge %s failed: %v", cfg.MergeAddress, err)
			for _, c := range serverConns {
				c.Close()
			}
			return
		}
		dlHeader := make([]byte, 12)
		copy(dlHeader[0:4], []byte("DLCH"))
		copy(dlHeader[4:12], aggSession[:])
		if _, err := mergeConn.Write(dlHeader); err != nil {
			log.Warnln("[ShieldLink] aggregate: send download header failed: %v", err)
			mergeConn.Close()
			for _, c := range serverConns {
				c.Close()
			}
			return
		}
		log.Infoln("[ShieldLink] aggregate: %d paths -> merge %s, session=%x", len(serverConns), cfg.MergeAddress, aggSession)
	} else {
		// Server-side merge: download channel goes through a decrypt server
		// The DLCH header is sent as initialData in the auth, relayed to merge by the server
		ep := pool.Pick()
		if ep == nil {
			log.Warnln("[ShieldLink] aggregate: no server for download channel")
			for _, c := range serverConns {
				c.Close()
			}
			return
		}
		var err error
		mergeConn, err = pool.DialDownloadChannel(ep, aggSession)
		if err != nil {
			log.Warnln("[ShieldLink] aggregate: download channel via %s failed: %v", ep.Address, err)
			for _, c := range serverConns {
				c.Close()
			}
			return
		}
		log.Infoln("[ShieldLink] aggregate: %d paths -> server-side merge, session=%x", len(serverConns), aggSession)
	}

	writer := NewAggregateWriter(aggSession, serverConns, 0)
	defer writer.Close()
	defer mergeConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// local → split → servers → merge (upload)
	go func() {
		defer wg.Done()
		io.Copy(writer, local)
	}()

	// merge → local (download)
	go func() {
		defer wg.Done()
		io.Copy(local, mergeConn)
	}()

	wg.Wait()
}

func relay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(b, a)
		if tc, ok := b.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		io.Copy(a, b)
		if tc, ok := a.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	wg.Wait()
}
