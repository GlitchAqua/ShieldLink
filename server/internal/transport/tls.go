package transport

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"
)

// NewTLSListener creates a TLS listener with auto-generated or provided certificate.
// If mptcp is true, uses MPTCP socket (Linux 5.6+).
func NewTLSListener(addr string, certPath, keyPath string, autoCert bool, mptcp bool) (net.Listener, error) {
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

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{"h2", "http/1.1"},
	}

	var ln net.Listener
	if mptcp {
		lc := &net.ListenConfig{}
		SetMPTCP(lc)
		tcpLn, err := lc.Listen(nil, "tcp", addr)
		if err != nil {
			return nil, err
		}
		ln = tls.NewListener(tcpLn, tlsCfg)
	} else {
		ln, err = tls.Listen("tcp", addr, tlsCfg)
		if err != nil {
			return nil, err
		}
	}

	return ln, nil
}

func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "shieldlink"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}, nil
}
