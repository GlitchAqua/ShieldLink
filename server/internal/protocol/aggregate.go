package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

// Aggregate frame format:
//   SESSION_ID (8) + SEQ_NUM (4) + CHUNK_LEN (2) + DATA (var)
// Total header: 14 bytes

const (
	AggSessionSize  = 8
	AggSeqSize      = 4
	AggChunkLenSize = 2
	AggHeaderSize   = AggSessionSize + AggSeqSize + AggChunkLenSize
	MaxChunkSize    = 32 * 1024 // 32KB max per chunk
)

// AggFrame represents a single aggregate frame.
type AggFrame struct {
	SessionID [AggSessionSize]byte
	SeqNum    uint32
	Data      []byte
}

// MarshalAggFrame encodes an aggregate frame to bytes.
func MarshalAggFrame(f *AggFrame) []byte {
	buf := make([]byte, AggHeaderSize+len(f.Data))
	copy(buf[0:AggSessionSize], f.SessionID[:])
	binary.BigEndian.PutUint32(buf[AggSessionSize:AggSessionSize+AggSeqSize], f.SeqNum)
	binary.BigEndian.PutUint16(buf[AggSessionSize+AggSeqSize:AggHeaderSize], uint16(len(f.Data)))
	copy(buf[AggHeaderSize:], f.Data)
	return buf
}

// ReadAggFrame reads one aggregate frame from a reader.
func ReadAggFrame(r io.Reader) (*AggFrame, error) {
	hdr := make([]byte, AggHeaderSize)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}

	f := &AggFrame{}
	copy(f.SessionID[:], hdr[0:AggSessionSize])
	f.SeqNum = binary.BigEndian.Uint32(hdr[AggSessionSize : AggSessionSize+AggSeqSize])
	chunkLen := binary.BigEndian.Uint16(hdr[AggSessionSize+AggSeqSize : AggHeaderSize])

	if chunkLen > MaxChunkSize {
		return nil, errors.New("chunk too large")
	}

	f.Data = make([]byte, chunkLen)
	if _, err := io.ReadFull(r, f.Data); err != nil {
		return nil, err
	}

	return f, nil
}

// WriteAggFrame writes one aggregate frame to a writer.
func WriteAggFrame(w io.Writer, f *AggFrame) error {
	_, err := w.Write(MarshalAggFrame(f))
	return err
}
