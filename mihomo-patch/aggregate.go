package shieldlink

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/log"
)

// Aggregate frame: SESSION_ID(8) + SEQ_NUM(4) + CHUNK_LEN(2) + DATA(var)
const (
	AggSessionSize  = 8
	AggSeqSize      = 4
	AggChunkLenSize = 2
	AggHeaderSize   = AggSessionSize + AggSeqSize + AggChunkLenSize
	DefaultChunkSize = 16 * 1024 // 16KB default chunk
)

// AggregateWriter splits a byte stream into aggregate frames and distributes
// them round-robin across multiple server connections.
type AggregateWriter struct {
	sessionID [AggSessionSize]byte
	conns     []net.Conn     // connections to ShieldLink servers
	seqNum    atomic.Uint32
	idx       atomic.Uint32  // round-robin index
	chunkSize int
	mu        sync.Mutex
}

// NewAggregateWriter creates a splitter that distributes chunks across conns.
func NewAggregateWriter(sessionID [AggSessionSize]byte, conns []net.Conn, chunkSize int) *AggregateWriter {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	return &AggregateWriter{
		sessionID: sessionID,
		conns:     conns,
		chunkSize: chunkSize,
	}
}

// Write splits data into chunks, wraps each in an aggregate frame,
// and sends them round-robin across server connections.
func (w *AggregateWriter) Write(data []byte) (int, error) {
	total := 0
	for len(data) > 0 {
		chunk := data
		if len(chunk) > w.chunkSize {
			chunk = data[:w.chunkSize]
		}
		data = data[len(chunk):]

		seq := w.seqNum.Add(1) - 1
		frame := marshalAggFrame(w.sessionID, seq, chunk)

		// Round-robin select connection
		idx := int(w.idx.Add(1)-1) % len(w.conns)
		conn := w.conns[idx]

		w.mu.Lock()
		_, err := conn.Write(frame)
		w.mu.Unlock()

		if err != nil {
			log.Debugln("[ShieldLink] aggregate write error on conn %d: %v", idx, err)
			return total, err
		}
		total += len(chunk)
	}
	return total, nil
}

// Close closes all underlying connections.
func (w *AggregateWriter) Close() error {
	for _, c := range w.conns {
		c.Close()
	}
	return nil
}

// AggregateReader reads reassembled data from a merge server connection.
type AggregateReader struct {
	conn net.Conn
}

func NewAggregateReader(conn net.Conn) *AggregateReader {
	return &AggregateReader{conn: conn}
}

func (r *AggregateReader) Read(buf []byte) (int, error) {
	return r.conn.Read(buf)
}

func (r *AggregateReader) Close() error {
	return r.conn.Close()
}

// AggregateConn wraps aggregate writer + reader as a net.Conn.
type AggregateConn struct {
	writer *AggregateWriter
	reader *AggregateReader
	local  net.Addr
	remote net.Addr
}

func NewAggregateConn(writer *AggregateWriter, reader *AggregateReader, local, remote net.Addr) *AggregateConn {
	return &AggregateConn{writer: writer, reader: reader, local: local, remote: remote}
}

func (c *AggregateConn) Read(b []byte) (int, error)  { return c.reader.Read(b) }
func (c *AggregateConn) Write(b []byte) (int, error) { return c.writer.Write(b) }
func (c *AggregateConn) Close() error {
	c.writer.Close()
	return c.reader.Close()
}
func (c *AggregateConn) LocalAddr() net.Addr                { return c.local }
func (c *AggregateConn) RemoteAddr() net.Addr               { return c.remote }
func (c *AggregateConn) SetDeadline(t time.Time) error      { return nil }
func (c *AggregateConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *AggregateConn) SetWriteDeadline(t time.Time) error { return nil }

func marshalAggFrame(sessionID [AggSessionSize]byte, seqNum uint32, data []byte) []byte {
	buf := make([]byte, AggHeaderSize+len(data))
	copy(buf[0:AggSessionSize], sessionID[:])
	binary.BigEndian.PutUint32(buf[AggSessionSize:AggSessionSize+AggSeqSize], seqNum)
	binary.BigEndian.PutUint16(buf[AggSessionSize+AggSeqSize:AggHeaderSize], uint16(len(data)))
	copy(buf[AggHeaderSize:], data)
	return buf
}

// ReadAggFrame reads one aggregate frame from a reader (used by merge server).
func ReadAggFrame(r io.Reader) (sessionID [AggSessionSize]byte, seqNum uint32, data []byte, err error) {
	hdr := make([]byte, AggHeaderSize)
	if _, err = io.ReadFull(r, hdr); err != nil {
		return
	}
	copy(sessionID[:], hdr[0:AggSessionSize])
	seqNum = binary.BigEndian.Uint32(hdr[AggSessionSize : AggSessionSize+AggSeqSize])
	chunkLen := binary.BigEndian.Uint16(hdr[AggSessionSize+AggSeqSize : AggHeaderSize])
	data = make([]byte, chunkLen)
	_, err = io.ReadFull(r, data)
	return
}
