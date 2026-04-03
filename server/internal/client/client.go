package client

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"shieldlink-server/internal/auth"
	"shieldlink-server/internal/config"
	"shieldlink-server/internal/log"
	"shieldlink-server/internal/relay"
)

type Client struct {
	cfg     *config.Config
	servers []*endpoint
}

type endpoint struct {
	address string
	alive   atomic.Bool
	latency atomic.Int64
}

func New(cfg *config.Config) *Client {
	c := &Client{cfg: cfg}
	for _, s := range cfg.Servers {
		if !s.Enabled {
			continue
		}
		ep := &endpoint{address: s.Address}
		ep.alive.Store(true)
		c.servers = append(c.servers, ep)
	}
	return c
}

func (c *Client) Run() error {
	ln, err := net.Listen("tcp", c.cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	log.L.Info("client started",
		"listen", c.cfg.Listen,
		"uuid", c.cfg.UUID[:8]+"...",
		"servers", len(c.servers),
	)
	for _, ep := range c.servers {
		log.L.Info("  server", "address", ep.address)
	}

	go c.healthCheckLoop()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.L.Error("accept error", "err", err)
			continue
		}
		go c.handleConn(conn)
	}
}

func (c *Client) handleConn(local net.Conn) {
	defer local.Close()

	ep := c.pick()
	if ep == nil {
		log.L.Error("no alive server")
		return
	}

	tunnel, err := c.dialTunnel(ep)
	if err != nil {
		// Try fallback
		ep2 := c.pick()
		if ep2 == nil || ep2.address == ep.address {
			log.L.Error("all servers failed", "last_err", err)
			return
		}
		tunnel, err = c.dialTunnel(ep2)
		if err != nil {
			log.L.Error("fallback failed", "err", err)
			return
		}
		ep = ep2
	}
	defer tunnel.Close()

	log.L.Debug("relay", "client", local.RemoteAddr(), "server", ep.address)
	upload, download := relay.TCPRelay(local, tunnel, nil)
	log.L.Debug("done", "server", ep.address, "up", upload, "down", download)
}

func (c *Client) dialTunnel(ep *endpoint) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	raw, err := dialer.Dial("tcp", ep.address)
	if err != nil {
		ep.alive.Store(false)
		return nil, err
	}

	tlsConn := tls.Client(raw, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{"h2", "http/1.1"},
	})
	if err := tlsConn.Handshake(); err != nil {
		raw.Close()
		ep.alive.Store(false)
		return nil, err
	}

	var sessionID [auth.SessionSize]byte
	rand.Read(sessionID[:])
	var flags byte
	if c.cfg.IPPassthrough {
		flags |= auth.FlagIPPassthrough
	}

	header := auth.BuildHeader(c.cfg.UUID, flags, sessionID, nil)
	if _, err := tlsConn.Write(header); err != nil {
		tlsConn.Close()
		return nil, err
	}

	return tlsConn, nil
}

func (c *Client) pick() *endpoint {
	var alive []*endpoint
	for _, ep := range c.servers {
		if ep.alive.Load() {
			alive = append(alive, ep)
		}
	}
	if len(alive) == 0 {
		if len(c.servers) > 0 {
			return c.servers[0]
		}
		return nil
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alive))))
	return alive[n.Int64()]
}

func (c *Client) healthCheckLoop() {
	time.Sleep(3 * time.Second)
	for {
		var wg sync.WaitGroup
		for _, ep := range c.servers {
			wg.Add(1)
			go func(ep *endpoint) {
				defer wg.Done()
				start := time.Now()
				conn, err := c.dialTunnel(ep)
				if err != nil {
					if ep.alive.Swap(false) {
						log.L.Warn("server DOWN", "addr", ep.address, "err", err)
					}
					return
				}
				conn.Close()
				ep.latency.Store(time.Since(start).Nanoseconds())
				if !ep.alive.Swap(true) {
					log.L.Info("server UP", "addr", ep.address)
				}
			}(ep)
		}
		wg.Wait()
		time.Sleep(30 * time.Second)
	}
}
