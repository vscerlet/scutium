package scutium

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type HandlerFunc func(ctx context.Context, conn net.Conn, payload []byte) error

type Server struct {
	addr        string
	protocol    string
	handlers    map[uint32]HandlerFunc
	listener    net.Listener
	wg          sync.WaitGroup
	IdleTimeout time.Duration // Timeout for conn.Read
}

func NewServer(addr string, protocol string) *Server {
	s := &Server{
		handlers:    make(map[uint32]HandlerFunc),
		addr:        addr,
		protocol:    protocol,
		IdleTimeout: 200 * time.Millisecond,
	}
	l, err := net.Listen(protocol, addr)
	if err != nil {
		log.Fatal(err)
	}
	s.listener = l
	return s
}

func (s *Server) On(pkgType uint32, handler HandlerFunc) {
	s.handlers[pkgType] = handler
}

// stop stops server, closes listener, waits for gouroutines, handler os signals
func (s *Server) stop(ctx context.Context, cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-sigChan:
			cancel()
			s.listener.Close()
			s.wg.Wait()
			log.Printf("Сервер благополучно выполнил graceful shutdown")
			return
		case <-ctx.Done():
			s.listener.Close()
			s.wg.Wait()
			log.Printf("Сервер благополучно выполнил graceful shutdown")
			return
		}
	}
}

func (s *Server) Listen() error {
	log.Printf("Сервер запущен и слушает на %s\n", s.addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Printf("Listen завершил работу")
				return
			default:
				conn, err := s.listener.Accept()
				if err != nil {
					// Check if listener is close then go to next iteration to handle context cancellation
					if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
						continue
					}
					return
				}
				s.wg.Add(1)
				go func() {
					s.handleConnection(ctx, conn)
					s.wg.Done()
				}()
			}
		}
	}()
	s.stop(ctx, cancel) // Waits for ctx cancel or SIGINT or SIGTERM
	return nil
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

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	log.Printf("Новое соединение от %s\n", conn.RemoteAddr())

	buffer := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			log.Printf("handle conn завершил работу")
			return
		default:
			// Timeout for conn.Read in order to jump to next iteration of for to check if ctx.Done
			conn.SetReadDeadline(time.Now().Add(s.IdleTimeout))
			n, err := conn.Read(buffer)
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() { // Check if timeout ender
					continue // Check context again
				}
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

			s.wg.Add(1)
			go func() {
				handler(ctx, conn, pkg.Payload)
				s.wg.Done()
			}()
		}
	}
}
