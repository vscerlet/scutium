package scutium

import (
	"encoding/binary"
	"io"
	"log"
	"net"
)

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

// TODO: Реализовать парсинг стандартного пакета
// TODO: Реализовать поиск обработчика и ошибку в случае её отсуствия
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

		// Пропускаем пустой пакет
		if n == 0 {
			continue
		}

		pkgID := binary.BigEndian.Uint32(buffer[:4])
		payload := buffer[4:n]

		handler, ok := s.handlers[pkgID]
		if !ok {
			log.Printf("Получен пакет с неизвестный ID - %d", pkgID)
			continue
		}
		go handler(conn, payload)
	}
}
