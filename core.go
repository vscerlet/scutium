package scutium

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
)

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

type HandlerFunc func(conn net.Conn, payload []byte) error

type Server struct {
	addr     string
	protocol string
	handlers map[uint32]HandlerFunc
}

func NewServer(addr string, protocol string) *Server {
	return &Server{addr: addr, protocol: protocol, handlers: make(map[uint32]HandlerFunc)}
}

func (s *Server) On(pkgType uint32, handler HandlerFunc) {
	s.handlers[pkgType] = handler
}

func (s *Server) Listen() error {
	listener, err := net.Listen(s.protocol, s.addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("Сервер запущен и слушает на %s\n", s.addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Ошибка при принятии соединения: %v\n", err)
		}

		go s.handleConnection(conn)
	}
}

func SendPkg(conn net.Conn, pkgID uint32, payload []byte) (int, error) {
	const op = "SendPkg"
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, pkgID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	buf.Write(payload)
	n, err := conn.Write(buf.Bytes())
	if err != nil {
		return n, fmt.Errorf("%s: %w", op, err)
	}
	return n, nil
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("Новое соединение от %s\n", conn.RemoteAddr())

	buffer := make([]byte, 1024)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				log.Printf("Соединение с %s закрыто\n", conn.RemoteAddr())
			} else {
				log.Printf("Ошибка чтения от %s: %v\n", conn.RemoteAddr(), err)
			}
			return
		}

		if n == 0 {
			continue
		}

		// Unpacking the package
		pkg := parseBasePacket(buffer[:n])

		handler, ok := s.handlers[pkg.ID]
		if !ok {
			log.Printf("Получен пакет с неизвестный ID - %d", pkg.ID)
			continue
		}
		go handler(conn, pkg.Payload)
	}
}
