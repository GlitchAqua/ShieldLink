package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
	"time"
)

func cryptoRandRead(b []byte) {
	rand.Read(b)
}

const (
	NonceWindow    = 120 * time.Second
	ReplayCacheTTL = 240 * time.Second

	KeyHintSize = 4
	HMACSize    = 32
	NonceSize   = 8
	PadLenSize  = 2
	FlagsSize   = 1
	SessionSize = 8

	// MinHeaderSize = KEY_HINT(4) + HMAC(32) + NONCE(8) + PAD_LEN(2) + FLAGS(1) + SESSION_ID(8) = 55
	MinHeaderSize = KeyHintSize + HMACSize + NonceSize + PadLenSize + FlagsSize + SessionSize
)

// Flags bits
const (
	FlagIPPassthrough = 1 << 0
	FlagAggregate     = 1 << 1
	FlagUDPOverTCP    = 1 << 2
)

// DerivedKey holds precomputed keys from a UUID.
type DerivedKey struct {
	MasterKey [32]byte
	KeyHint   [KeyHintSize]byte
}

// DeriveKey computes the master key and hint from a UUID string.
func DeriveKey(uuid string) DerivedKey {
	master := sha256.Sum256([]byte(uuid))
	var dk DerivedKey
	dk.MasterKey = master
	copy(dk.KeyHint[:], master[:KeyHintSize])
	return dk
}

// Header represents a parsed ShieldLink authentication header.
type Header struct {
	Flags       byte
	SessionID   [SessionSize]byte
	InitialData []byte
}

// Authenticator handles HMAC verification and replay prevention.
type Authenticator struct {
	key         DerivedKey
	mu          sync.Mutex
	replayCache map[[HMACSize]byte]time.Time
}

func NewAuthenticator(uuid string) *Authenticator {
	a := &Authenticator{
		key:         DeriveKey(uuid),
		replayCache: make(map[[HMACSize]byte]time.Time),
	}
	go a.cleanupLoop()
	return a
}

func (a *Authenticator) KeyHint() [KeyHintSize]byte {
	return a.key.KeyHint
}

// Verify parses and verifies an incoming authentication header.
// Returns the parsed header on success.
func (a *Authenticator) Verify(data []byte) (*Header, error) {
	if len(data) < MinHeaderSize {
		return nil, errors.New("data too short")
	}
	pos := 0

	// KEY_HINT
	var hint [KeyHintSize]byte
	copy(hint[:], data[pos:pos+KeyHintSize])
	pos += KeyHintSize

	if hint != a.key.KeyHint {
		return nil, errors.New("key hint mismatch")
	}

	// HMAC
	var receivedHMAC [HMACSize]byte
	copy(receivedHMAC[:], data[pos:pos+HMACSize])
	pos += HMACSize

	// NONCE
	var nonce [NonceSize]byte
	copy(nonce[:], data[pos:pos+NonceSize])
	pos += NonceSize

	// Verify timestamp (upper 4 bytes are seconds)
	ts := int64(binary.BigEndian.Uint32(nonce[:4]))
	now := time.Now().Unix()
	diff := now - ts
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(NonceWindow.Seconds()) {
		return nil, errors.New("nonce expired")
	}

	// Verify HMAC
	mac := hmac.New(sha256.New, a.key.MasterKey[:])
	mac.Write(nonce[:])
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, receivedHMAC[:]) {
		return nil, errors.New("hmac mismatch")
	}

	// Replay check
	a.mu.Lock()
	if _, exists := a.replayCache[receivedHMAC]; exists {
		a.mu.Unlock()
		return nil, errors.New("replay detected")
	}
	a.replayCache[receivedHMAC] = time.Now().Add(ReplayCacheTTL)
	a.mu.Unlock()

	// PAD_LEN
	if pos+PadLenSize > len(data) {
		return nil, errors.New("data too short for pad_len")
	}
	padLen := int(binary.BigEndian.Uint16(data[pos : pos+PadLenSize]))
	pos += PadLenSize

	// Skip padding
	if padLen > 900 {
		return nil, errors.New("padding too large")
	}
	if pos+padLen > len(data) {
		return nil, errors.New("data too short for padding")
	}
	pos += padLen

	// FLAGS
	if pos+FlagsSize > len(data) {
		return nil, errors.New("data too short for flags")
	}
	flags := data[pos]
	pos += FlagsSize

	// SESSION_ID
	if pos+SessionSize > len(data) {
		return nil, errors.New("data too short for session_id")
	}
	var sessionID [SessionSize]byte
	copy(sessionID[:], data[pos:pos+SessionSize])
	pos += SessionSize

	// INITIAL_DATA
	var initialData []byte
	if pos < len(data) {
		initialData = make([]byte, len(data)-pos)
		copy(initialData, data[pos:])
	}

	return &Header{
		Flags:       flags,
		SessionID:   sessionID,
		InitialData: initialData,
	}, nil
}

// BuildHeader constructs an authentication header for the client side.
func BuildHeader(uuid string, flags byte, sessionID [SessionSize]byte, initialData []byte) []byte {
	dk := DeriveKey(uuid)

	// NONCE = upper 4 bytes timestamp (seconds) + lower 4 bytes random
	var nonce [NonceSize]byte
	binary.BigEndian.PutUint32(nonce[:4], uint32(time.Now().Unix()))
	cryptoRandRead(nonce[4:8])

	// HMAC
	mac := hmac.New(sha256.New, dk.MasterKey[:])
	mac.Write(nonce[:])
	hmacVal := mac.Sum(nil)

	// Random padding (64-256 bytes for lightweight)
	padLen := 64 + int(nonce[0])%193 // deterministic-ish from nonce, 64-256
	padding := make([]byte, padLen)

	// Build header
	buf := make([]byte, 0, MinHeaderSize+PadLenSize+padLen+len(initialData))
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

func (a *Authenticator) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		a.mu.Lock()
		now := time.Now()
		for k, expiry := range a.replayCache {
			if now.After(expiry) {
				delete(a.replayCache, k)
			}
		}
		a.mu.Unlock()
	}
}
