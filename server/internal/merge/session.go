package merge

import (
	"io"
	"net"
	"sync"
	"time"

	"shieldlink-server/internal/log"
	"shieldlink-server/internal/protocol"
)

// Session tracks one aggregate session (one client connection).
// It collects chunks from multiple server paths and reassembles them in order.
type Session struct {
	id           [protocol.AggSessionSize]byte
	target       net.Conn // connection to the proxy server
	downloadConn net.Conn // download channel back to client
	mu           sync.Mutex
	nextSeq      uint32
	buf          map[uint32][]byte // out-of-order buffer
	lastSeen     time.Time
	closed       bool
}

func newSession(id [protocol.AggSessionSize]byte, target net.Conn) *Session {
	return &Session{
		id:       id,
		target:   target,
		buf:      make(map[uint32][]byte),
		lastSeen: time.Now(),
	}
}

// Push adds a chunk. If it's the next expected sequence, flushes all consecutive chunks.
func (s *Session) Push(frame *protocol.AggFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.lastSeen = time.Now()

	if frame.SeqNum == s.nextSeq {
		// In order - write directly
		s.writeToTarget(frame.Data)
		s.nextSeq++

		// Flush any buffered consecutive chunks
		for {
			data, ok := s.buf[s.nextSeq]
			if !ok {
				break
			}
			s.writeToTarget(data)
			delete(s.buf, s.nextSeq)
			s.nextSeq++
		}
	} else if frame.SeqNum > s.nextSeq {
		// Out of order - buffer it
		s.buf[frame.SeqNum] = frame.Data
		log.L.Debug("buffered out-of-order chunk",
			"session", frame.SessionID,
			"seq", frame.SeqNum,
			"expected", s.nextSeq,
			"buffered", len(s.buf),
		)
	}
	// seq < nextSeq is a duplicate, ignore
}

func (s *Session) writeToTarget(data []byte) {
	if _, err := s.target.Write(data); err != nil {
		log.L.Error("write to target failed", "err", err)
		s.closed = true
	}
}

// SetDownloadConn sets the download channel and starts relaying responses.
func (s *Session) SetDownloadConn(conn net.Conn) {
	s.mu.Lock()
	s.downloadConn = conn
	target := s.target
	s.mu.Unlock()

	if target != nil {
		// Relay: target (SS proxy response) → download conn (back to client)
		go func() {
			io.Copy(conn, target)
			conn.Close()
		}()
	}
}

// Close closes the target and download connections.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.target != nil {
		s.target.Close()
	}
	if s.downloadConn != nil {
		s.downloadConn.Close()
	}
}

// IsStale returns true if the session has not seen traffic for the given timeout.
func (s *Session) IsStale(timeout time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastSeen) > timeout
}
