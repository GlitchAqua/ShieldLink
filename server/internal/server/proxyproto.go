package server

import (
	"encoding/binary"
	"net"
)

// PROXY Protocol v2 signature
var proxyV2Sig = [12]byte{0x0D, 0x0A, 0x0D, 0x0A, 0x00, 0x0D, 0x0A, 0x51, 0x55, 0x49, 0x54, 0x0A}

// BuildProxyProtocolV2 constructs a PROXY Protocol v2 header.
// command: 0x21 = PROXY (proxied connection)
// family:  0x11 = AF_INET + STREAM, 0x21 = AF_INET6 + STREAM
func BuildProxyProtocolV2(srcAddr, dstAddr net.Addr) []byte {
	srcTCP, ok1 := srcAddr.(*net.TCPAddr)
	dstTCP, ok2 := dstAddr.(*net.TCPAddr)
	if !ok1 || !ok2 {
		return nil
	}

	srcIP := srcTCP.IP.To4()
	dstIP := dstTCP.IP.To4()

	isV6 := false
	if srcIP == nil {
		srcIP = srcTCP.IP.To16()
		dstIP = dstTCP.IP.To16()
		isV6 = true
	}
	if dstIP == nil && !isV6 {
		dstIP = dstTCP.IP.To16()
		srcIP = srcTCP.IP.To16()
		isV6 = true
	}

	var family byte
	var addrLen int
	if isV6 {
		family = 0x21 // AF_INET6 + STREAM
		addrLen = 16 + 16 + 2 + 2
	} else {
		family = 0x11 // AF_INET + STREAM
		addrLen = 4 + 4 + 2 + 2
	}

	buf := make([]byte, 16+addrLen)
	copy(buf[0:12], proxyV2Sig[:])
	buf[12] = 0x21 // version 2 | PROXY command
	buf[13] = family
	binary.BigEndian.PutUint16(buf[14:16], uint16(addrLen))

	pos := 16
	copy(buf[pos:], srcIP)
	pos += len(srcIP)
	copy(buf[pos:], dstIP)
	pos += len(dstIP)
	binary.BigEndian.PutUint16(buf[pos:], uint16(srcTCP.Port))
	pos += 2
	binary.BigEndian.PutUint16(buf[pos:], uint16(dstTCP.Port))

	return buf
}
