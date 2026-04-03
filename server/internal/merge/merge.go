package merge

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"shieldlink-server/internal/config"
	"shieldlink-server/internal/log"
	"shieldlink-server/internal/protocol"
)

// Download channel magic: "DLCH" (4 bytes) + SESSION_ID (8 bytes) = 12 bytes
var downloadMagic = [4]byte{'D', 'L', 'C', 'H'}

// Merge server receives aggregate frames from multiple ShieldLink servers,
// reassembles them by SESSION_ID and SEQ_NUM, and forwards to the proxy server.
type Merge struct {
	cfg              *config.Config
	sessions         sync.Map // map[[AggSessionSize]byte]*Session
	pendingDownloads sync.Map // map[[AggSessionSize]byte]net.Conn
	timeout          time.Duration
}

func New(cfg *config.Config) *Merge {
	timeout := time.Duration(cfg.Reassembly.Timeout) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &Merge{
		cfg:     cfg,
		timeout: timeout,
	}
}

func (m *Merge) Run() error {
	// Merge listens on plain TCP - server-to-merge is internal traffic.
	// Auth is still required to validate legitimate server connections.
	ln, err := net.Listen("tcp", m.cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	log.L.Info("merge server started",
		"listen", m.cfg.Listen,
		"forward", m.cfg.Forward,
		"timeout", m.timeout,
	)

	// Cleanup stale sessions periodically
	go m.cleanupLoop()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.L.Error("accept error", "err", err)
			continue
		}
		go m.handleConn(conn)
	}
}

func (m *Merge) handleConn(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	log.L.Debug("merge: new connection", "remote", remoteAddr)

	// Read first 4 bytes to detect connection type
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	peek := make([]byte, 4)
	if _, err := io.ReadFull(conn, peek); err != nil {
		log.L.Debug("merge: peek error", "remote", remoteAddr, "err", err)
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	// Check if this is a download channel ("DLCH" magic)
	if peek[0] == downloadMagic[0] && peek[1] == downloadMagic[1] &&
		peek[2] == downloadMagic[2] && peek[3] == downloadMagic[3] {
		m.handleDownloadChannel(conn, remoteAddr)
		return
	}

	// Otherwise: server relay connection (aggregate frames)
	// The first 4 bytes we read are the start of the first aggregate frame's SESSION_ID
	defer conn.Close()
	m.handleServerRelay(conn, peek, remoteAddr)
}

func (m *Merge) handleDownloadChannel(conn net.Conn, remoteAddr string) {
	// Read SESSION_ID (8 bytes)
	var sessionID [protocol.AggSessionSize]byte
	if _, err := io.ReadFull(conn, sessionID[:]); err != nil {
		log.L.Debug("merge: download channel read session failed", "remote", remoteAddr, "err", err)
		conn.Close()
		return
	}

	log.L.Info("merge: download channel registered", "remote", remoteAddr, "session", fmt.Sprintf("%x", sessionID))

	// Find or wait for session, then set download conn
	// The session might not exist yet (race with first aggregate frame)
	// Store in a pending map, session will pick it up
	m.pendingDownloads.Store(sessionID, conn)

	// Also check if session already exists
	if s, ok := m.sessions.Load(sessionID); ok {
		sess := s.(*Session)
		if dc, loaded := m.pendingDownloads.LoadAndDelete(sessionID); loaded {
			sess.SetDownloadConn(dc.(net.Conn))
		}
	}
	// If session doesn't exist yet, getOrCreateSession will check pendingDownloads
}

func (m *Merge) handleServerRelay(conn net.Conn, firstBytes []byte, remoteAddr string) {
	log.L.Info("merge: server relay connection", "remote", remoteAddr)

	// Read rest of first frame header: we have 4 bytes, need 10 more (total 14 = AggHeaderSize)
	remaining := make([]byte, protocol.AggHeaderSize-4)
	if _, err := io.ReadFull(conn, remaining); err != nil {
		log.L.Debug("merge: read first frame header failed", "err", err)
		return
	}

	// Parse first frame manually
	hdr := append(firstBytes, remaining...)
	var sessionID [protocol.AggSessionSize]byte
	copy(sessionID[:], hdr[0:protocol.AggSessionSize])
	seqNum := binary.BigEndian.Uint32(hdr[protocol.AggSessionSize : protocol.AggSessionSize+protocol.AggSeqSize])
	chunkLen := binary.BigEndian.Uint16(hdr[protocol.AggSessionSize+protocol.AggSeqSize : protocol.AggHeaderSize])

	if int(chunkLen) > protocol.MaxChunkSize {
		log.L.Warn("merge: first chunk too large", "len", chunkLen)
		return
	}

	data := make([]byte, chunkLen)
	if _, err := io.ReadFull(conn, data); err != nil {
		log.L.Debug("merge: read first chunk data failed", "err", err)
		return
	}

	// Process first frame
	session := m.getOrCreateSession(sessionID)
	session.Push(&protocol.AggFrame{SessionID: sessionID, SeqNum: seqNum, Data: data})

	// Continue reading subsequent frames
	for {
		frame, err := protocol.ReadAggFrame(conn)
		if err != nil {
			log.L.Debug("merge: read frame done", "remote", remoteAddr, "err", err)
			return
		}

		session := m.getOrCreateSession(frame.SessionID)
		session.Push(frame)
	}
}

func (m *Merge) getOrCreateSession(id [protocol.AggSessionSize]byte) *Session {
	if s, ok := m.sessions.Load(id); ok {
		return s.(*Session)
	}

	// New session: connect to proxy server
	target, err := net.DialTimeout("tcp", m.cfg.Forward, 10*time.Second)
	if err != nil {
		log.L.Error("merge: dial forward failed", "forward", m.cfg.Forward, "err", err)
		s := newSession(id, &discardConn{})
		m.sessions.Store(id, s)
		return s
	}

	log.L.Info("merge: new session", "session", fmt.Sprintf("%x", id), "forward", m.cfg.Forward)
	s := newSession(id, target)
	m.sessions.Store(id, s)

	// Check if a download channel is already pending for this session
	if dc, ok := m.pendingDownloads.LoadAndDelete(id); ok {
		log.L.Info("merge: attaching download channel", "session", fmt.Sprintf("%x", id))
		s.SetDownloadConn(dc.(net.Conn))
	}

	return s
}

func (m *Merge) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		m.sessions.Range(func(key, value any) bool {
			s := value.(*Session)
			if s.IsStale(m.timeout) {
				log.L.Info("merge: closing stale session", "session", fmt.Sprintf("%x", key))
				s.Close()
				m.sessions.Delete(key)
			}
			return true
		})
	}
}

// discardConn is a net.Conn that discards all writes.
type discardConn struct{}

func (d *discardConn) Write(b []byte) (int, error)        { return len(b), nil }
func (d *discardConn) Read(b []byte) (int, error)         { return 0, fmt.Errorf("closed") }
func (d *discardConn) Close() error                       { return nil }
func (d *discardConn) LocalAddr() net.Addr                { return nil }
func (d *discardConn) RemoteAddr() net.Addr               { return nil }
func (d *discardConn) SetDeadline(t time.Time) error      { return nil }
func (d *discardConn) SetReadDeadline(t time.Time) error  { return nil }
func (d *discardConn) SetWriteDeadline(t time.Time) error { return nil }
