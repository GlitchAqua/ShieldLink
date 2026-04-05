package shieldlink

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/component/resolver"
	"github.com/metacubex/mihomo/log"

	"github.com/metacubex/quic-go"
	mtls "github.com/metacubex/tls"
)

type Endpoint struct {
	Address string
	alive   atomic.Bool
	latency atomic.Int64
}

type Pool struct {
	endpoints []*Endpoint
	cfg       *ClientConfig
	closed    chan struct{}
}

func NewPool(cfg *ClientConfig) *Pool {
	p := &Pool{cfg: cfg, closed: make(chan struct{})}
	for _, s := range cfg.Servers {
		if !s.Enabled {
			continue
		}
		ep := &Endpoint{Address: s.Address}
		ep.alive.Store(true)
		p.endpoints = append(p.endpoints, ep)
	}
	go p.healthLoop()
	return p
}

func (p *Pool) Pick() *Endpoint {
	var alive []*Endpoint
	for _, ep := range p.endpoints {
		if ep.alive.Load() {
			alive = append(alive, ep)
		}
	}
	if len(alive) == 0 {
		if len(p.endpoints) > 0 {
			return p.endpoints[0]
		}
		return nil
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alive))))
	return alive[n.Int64()]
}

func (p *Pool) Close() {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
}

// DialTunnel creates a tunnel to a ShieldLink server endpoint (TCP/TLS or QUIC).
func (p *Pool) DialTunnel(ep *Endpoint) (net.Conn, error) {
	host, port, err := net.SplitHostPort(ep.Address)
	if err != nil {
		return nil, err
	}

	// Resolve via proxy-server-nameserver to avoid circular DNS
	var ip netip.Addr
	ip, err = resolver.ResolveIPWithResolver(context.Background(), host, resolver.ProxyServerHostResolver)
	if err != nil {
		ip, err = netip.ParseAddr(host)
		if err != nil {
			ep.alive.Store(false)
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
	}

	resolved := net.JoinHostPort(ip.String(), port)

	var conn net.Conn
	if p.cfg.Protocol == "udp" {
		conn, err = p.dialQUIC(resolved)
	} else {
		conn, err = p.dialTLS(resolved)
	}
	if err != nil {
		ep.alive.Store(false)
		return nil, err
	}

	// Send auth header
	var flags byte
	if p.cfg.IPPassthrough {
		flags |= FlagIPPassthrough
	}
	sessionID := NewSessionID()
	header := BuildHeader(p.cfg.UUID, flags, sessionID, nil)
	if _, err := conn.Write(header); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// DialDownloadChannel connects to a decrypt server and includes a DLCH download
// channel header as initialData, so the server relays it to the merge server.
func (p *Pool) DialDownloadChannel(ep *Endpoint, aggSessionID [SessionSize]byte) (net.Conn, error) {
	host, port, err := net.SplitHostPort(ep.Address)
	if err != nil {
		return nil, err
	}

	var ip netip.Addr
	ip, err = resolver.ResolveIPWithResolver(context.Background(), host, resolver.ProxyServerHostResolver)
	if err != nil {
		ip, err = netip.ParseAddr(host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
	}

	resolved := net.JoinHostPort(ip.String(), port)

	var conn net.Conn
	if p.cfg.Protocol == "udp" {
		conn, err = p.dialQUIC(resolved)
	} else {
		conn, err = p.dialTLS(resolved)
	}
	if err != nil {
		return nil, err
	}

	var flags byte
	if p.cfg.IPPassthrough {
		flags |= FlagIPPassthrough
	}

	// Build DLCH header (4 bytes magic + 8 bytes session ID) as initialData
	dlHeader := make([]byte, 12)
	copy(dlHeader[0:4], []byte("DLCH"))
	copy(dlHeader[4:12], aggSessionID[:])

	connSessionID := NewSessionID()
	header := BuildHeader(p.cfg.UUID, flags, connSessionID, dlHeader)
	if _, err := conn.Write(header); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func (p *Pool) dialTLS(resolved string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if p.cfg.MPTCP {
		dialer.SetMultipathTCP(true)
	}
	raw, err := dialer.Dial("tcp", resolved)
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(raw, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{"h2", "http/1.1"},
	})
	if err := tlsConn.Handshake(); err != nil {
		raw.Close()
		return nil, err
	}
	return tlsConn, nil
}

func (p *Pool) dialQUIC(resolved string) (net.Conn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", resolved)
	if err != nil {
		return nil, err
	}

	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	tr := &quic.Transport{Conn: udpConn}
	tr.SetCreatedConn(true)
	tr.SetSingleUse(true)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	qconn, err := tr.Dial(ctx, udpAddr, &mtls.Config{
		InsecureSkipVerify: true,
		MinVersion:         mtls.VersionTLS13,
		NextProtos:         []string{"shieldlink"},
	}, &quic.Config{
		MaxIdleTimeout:             30 * time.Second,
		InitialStreamReceiveWindow: 512 * 1024,
		MaxStreamReceiveWindow:     4 * 1024 * 1024,
	})
	if err != nil {
		udpConn.Close()
		return nil, err
	}

	stream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		qconn.CloseWithError(1, "open stream failed")
		return nil, err
	}

	return &quicStreamConn{
		Stream: stream,
		qconn:  qconn,
		local:  udpConn.LocalAddr(),
		remote: udpAddr,
	}, nil
}

// quicStreamConn wraps a *quic.Stream as net.Conn.
type quicStreamConn struct {
	*quic.Stream
	qconn  *quic.Conn
	local  net.Addr
	remote net.Addr
}

func (c *quicStreamConn) LocalAddr() net.Addr  { return c.local }
func (c *quicStreamConn) RemoteAddr() net.Addr { return c.remote }
func (c *quicStreamConn) Close() error {
	c.Stream.CancelRead(0)
	c.Stream.Close()
	return c.qconn.CloseWithError(0, "")
}

// DialWithFailover tries primary, falls back to another endpoint.
func (p *Pool) DialWithFailover() (net.Conn, error) {
	ep := p.Pick()
	if ep == nil {
		return nil, net.ErrClosed
	}
	conn, err := p.DialTunnel(ep)
	if err != nil {
		log.Warnln("[ShieldLink] %s failed: %v, trying fallback", ep.Address, err)
		ep2 := p.Pick()
		if ep2 == nil || ep2.Address == ep.Address {
			return nil, err
		}
		return p.DialTunnel(ep2)
	}
	return conn, nil
}

func (p *Pool) healthLoop() {
	time.Sleep(5 * time.Second)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.closed:
			return
		case <-ticker.C:
			var wg sync.WaitGroup
			for _, ep := range p.endpoints {
				wg.Add(1)
				go func(ep *Endpoint) {
					defer wg.Done()
					start := time.Now()
					conn, err := p.DialTunnel(ep)
					if err != nil {
						if ep.alive.Swap(false) {
							log.Warnln("[ShieldLink] server DOWN: %s (%v)", ep.Address, err)
						}
						return
					}
					conn.Close()
					ep.latency.Store(time.Since(start).Nanoseconds())
					if !ep.alive.Swap(true) {
						log.Infoln("[ShieldLink] server UP: %s", ep.Address)
					}
				}(ep)
			}
			wg.Wait()
		}
	}
}
