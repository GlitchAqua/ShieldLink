package relay

import (
	"io"
	"net"
	"sync"
)

// StreamRelay performs bidirectional relay between an io.ReadWriteCloser (QUIC stream)
// and a net.Conn (TCP target). Returns bytes transferred.
func StreamRelay(stream io.ReadWriteCloser, target net.Conn) (upload, download int64) {
	var wg sync.WaitGroup
	var ul, dl int64

	wg.Add(2)

	// stream → target (upload)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(target, stream)
		ul = n
		if tc, ok := target.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	// target → stream (download)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(stream, target)
		dl = n
		stream.Close()
	}()

	wg.Wait()
	return ul, dl
}
