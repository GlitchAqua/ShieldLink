package shieldlink

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

const (
	KeyHintSize = 4
	HMACSize    = 32
	NonceSize   = 8
	PadLenSize  = 2
	FlagsSize   = 1
	SessionSize = 8

	FlagIPPassthrough = 1 << 0
	FlagAggregate     = 1 << 1
	FlagUDPOverTCP    = 1 << 2
)

type DerivedKey struct {
	MasterKey [32]byte
	KeyHint   [KeyHintSize]byte
}

func DeriveKey(uuid string) DerivedKey {
	master := sha256.Sum256([]byte(uuid))
	var dk DerivedKey
	dk.MasterKey = master
	copy(dk.KeyHint[:], master[:KeyHintSize])
	return dk
}

// BuildHeader constructs an authentication header for sending to the server.
func BuildHeader(uuid string, flags byte, sessionID [SessionSize]byte, initialData []byte) []byte {
	dk := DeriveKey(uuid)

	// NONCE = upper 4 bytes timestamp (seconds) + lower 4 bytes random
	var nonce [NonceSize]byte
	binary.BigEndian.PutUint32(nonce[:4], uint32(time.Now().Unix()))
	rand.Read(nonce[4:8])

	mac := hmac.New(sha256.New, dk.MasterKey[:])
	mac.Write(nonce[:])
	hmacVal := mac.Sum(nil)

	// Random padding 64-256 bytes
	padLen := 64
	var rndByte [1]byte
	rand.Read(rndByte[:])
	padLen += int(rndByte[0]) % 193
	padding := make([]byte, padLen)
	rand.Read(padding)

	size := KeyHintSize + HMACSize + NonceSize + PadLenSize + padLen + FlagsSize + SessionSize + len(initialData)
	buf := make([]byte, 0, size)

	buf = append(buf, dk.KeyHint[:]...)
	buf = append(buf, hmacVal...)
	buf = append(buf, nonce[:]...)

	padLenBytes := make([]byte, PadLenSize)
	binary.BigEndian.PutUint16(padLenBytes, uint16(padLen))
	buf = append(buf, padLenBytes...)
	buf = append(buf, padding...)

	buf = append(buf, flags)
	buf = append(buf, sessionID[:]...)
	buf = append(buf, initialData...)

	return buf
}

// NewSessionID generates a random 8-byte session ID.
func NewSessionID() [SessionSize]byte {
	var id [SessionSize]byte
	rand.Read(id[:])
	return id
}
