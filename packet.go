package scutium

import (
	"encoding/binary"
)

// BasicPacket represents the basic structure of a data packet,
// consisting of a packet identifier (ID) and payload.
// The ID is stored as a 32-bit integer, and the payload contains the rest of the packet.
type BasicPacket struct {
	Length  uint32
	ID      uint32
	Payload []byte
}

// Parses the package into fields according to its structure
func parseBasicPacket(pkg []byte) *BasicPacket {
	pkgLength := binary.BigEndian.Uint32(pkg[:4])
	pkgID := binary.BigEndian.Uint32(pkg[4:8])
	payload := pkg[8:]
	return &BasicPacket{Length: pkgLength, ID: pkgID, Payload: payload}
}
