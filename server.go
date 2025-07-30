package scutium

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
)

type HandlerFunc func(conn net.Conn, pkg BasicPacket) error

type Server struct {
	addr          string
	protocol      string
	maxPacketSize uint32
	handlers      map[uint32]HandlerFunc
}

func NewServer(addr string, protocol string) *Server {
	return &Server{
		addr:          addr,
		protocol:      protocol,
		maxPacketSize: 1024 * 1024, // 1 MB
		handlers:      make(map[uint32]HandlerFunc),
	}
}

func (s *Server) SetMaxPacketSize(size uint32) {
	s.maxPacketSize = size
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

	for {
		// Read the beginning of the packet from the socket
		header := make([]byte, 4)
		if _, err := io.ReadFull(conn, header); err != nil {
			if err == io.EOF {
				log.Printf("Соединение с %s закрыто\n", conn.RemoteAddr())
			} else {
				log.Printf("Ошибка чтения заголовка от %s: %v\n", conn.RemoteAddr(), err)
			}
			return
		}

		// Extract the length of the packet
		pkgLength := binary.BigEndian.Uint32(header)

		// Skip incomplete packages
		if pkgLength < 8 {
			log.Printf("Некорректная длина пакета от %s: %d", conn.RemoteAddr(), pkgLength)
			return
		}

		if pkgLength > s.maxPacketSize {
			log.Printf("Пакет слишком большой от %s: %d байт, максимальный размер %d байт", conn.RemoteAddr(), pkgLength, s.maxPacketSize)
			return
		}

		// Read the rest of the packet (pkgLength - 4 bytes)
		body := make([]byte, pkgLength-4)
		if _, err := io.ReadFull(conn, body); err != nil {
			if err == io.EOF {
				log.Printf("Соединение с %s закрыто\n", conn.RemoteAddr())
			} else {
				log.Printf("Ошибка чтения пакета от %s: %v\n", conn.RemoteAddr(), err)
			}
			return
		}

		// Collect the complete package
		packet := append(header, body...)

		// Parsing packet fields
		pkg := parseBasicPacket(packet)

		// Get a handler for the package
		handler, ok := s.handlers[pkg.ID]
		if !ok {
			log.Printf("Получен пакет с неизвестный ID - %d", pkg.ID)
			continue
		}

		go handler(conn, *pkg)
	}
}
