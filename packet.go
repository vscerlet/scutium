package scutium

import (
	"encoding/binary"
)

// BasicPacket represents the basic structure of a data packet,
// consisting of a packet identifier (ID) and payload.
// The ID is stored as a 32-bit integer, and the payload contains the rest of the packet.
type BasicPacket struct {
	ID      uint32
	Payload []byte
}

// Parses the package into fields according to its structure
func parseBasicPacket(pkg []byte) *BasicPacket {
	pkgID := binary.BigEndian.Uint32(pkg[:4])
	payload := pkg[4:]

	return &BasicPacket{ID: pkgID, Payload: payload}
}
