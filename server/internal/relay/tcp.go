package relay

import (
	"io"
	"net"
	"sync"
)

// TCPRelay performs bidirectional TCP relay between client and target.
// Returns total bytes transferred (upload, download).
func TCPRelay(client, target net.Conn, initialPayload []byte) (upload, download int64) {
	if len(initialPayload) > 0 {
		n, err := target.Write(initialPayload)
		if err != nil {
			return 0, 0
		}
		upload = int64(n)
	}

	var wg sync.WaitGroup
	var ul, dl int64

	wg.Add(2)

	// client → target (upload)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(target, client)
		ul = n
		if tc, ok := target.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	// target → client (download)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(client, target)
		dl = n
		if tc, ok := client.(interface{ CloseWrite() error }); ok {
			tc.CloseWrite()
		}
	}()

	wg.Wait()
	return upload + ul, dl
}
