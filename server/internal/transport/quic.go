package transport

import (
	"crypto/tls"
	"net"
	"time"

	mtls "github.com/metacubex/tls"

	"github.com/metacubex/quic-go"
)

// NewQUICListener creates a QUIC listener for UDP mode.
func NewQUICListener(addr string, certPath, keyPath string, autoCert bool) (*quic.Listener, error) {
	var cert tls.Certificate
	var err error

	if autoCert || (certPath == "" && keyPath == "") {
		cert, err = generateSelfSignedCert()
		if err != nil {
			return nil, err
		}
	} else {
		cert, err = tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, err
		}
	}

	// Convert standard cert to metacubex/tls cert
	mCerts := make([]mtls.Certificate, len([]tls.Certificate{cert}))
	mCerts[0] = mtls.Certificate{
		Certificate: cert.Certificate,
		PrivateKey:  cert.PrivateKey,
	}

	tlsCfg := &mtls.Config{
		Certificates: mCerts,
		MinVersion:   mtls.VersionTLS13,
		NextProtos:   []string{"shieldlink"},
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	quicCfg := &quic.Config{
		MaxIdleTimeout:             30 * time.Second,
		MaxIncomingStreams:         1 << 32,
		InitialStreamReceiveWindow: 512 * 1024,
		MaxStreamReceiveWindow:     4 * 1024 * 1024,
		Allow0RTT:                  true,
	}

	ln, err := quic.Listen(udpConn, tlsCfg, quicCfg)
	if err != nil {
		udpConn.Close()
		return nil, err
	}

	return ln, nil
}
