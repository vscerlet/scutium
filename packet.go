package scutium

import "encoding/binary"

// basePacket represents the basic structure of a data packet,
// consisting of a packet identifier (ID) and payload.
// The ID is stored as a 32-bit integer, and the payload contains the rest of the packet.
type basePacket struct {
	ID      uint32
	Payload []byte
}

// Parses the package into fields according to its structure
func parseBasePacket(pkg []byte) *basePacket {
	pkgID := binary.BigEndian.Uint32(pkg[:4])
	payload := pkg[4:]

	return &basePacket{ID: pkgID, Payload: payload}
}
