package transport

import "net"

// SetMPTCP enables MPTCP on a ListenConfig (Go 1.21+, Linux 5.6+).
func SetMPTCP(lc *net.ListenConfig) {
	lc.SetMultipathTCP(true)
}

// SetDialerMPTCP enables MPTCP on a Dialer.
func SetDialerMPTCP(d *net.Dialer) {
	d.SetMultipathTCP(true)
}
