package scutium

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
)

type HandlerFunc func(ctx context.Context, conn net.Conn, pkg BasicPacket) error

type Server struct {
	addr          string
	protocol      string
	maxPacketSize uint32
	handlers      map[uint32]HandlerFunc
	exit          context.CancelFunc
	wg            sync.WaitGroup
	log           *slog.Logger
}

func NewServer(addr string, protocol string) *Server {
	return &Server{
		addr:          addr,
		protocol:      protocol,
		maxPacketSize: 1024 * 1024, // 1 MB
		handlers:      make(map[uint32]HandlerFunc),
		log:           slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
}

func (s *Server) SetLogger(log *slog.Logger) {
	s.log = log
}

func (s *Server) SetMaxPacketSize(size uint32) {
	s.maxPacketSize = size
}

func (s *Server) On(pkgType uint32, handler HandlerFunc) {
	s.handlers[pkgType] = handler
}

func (s *Server) Stop() {
	if s.exit != nil {
		s.exit()
	}
}

func (s *Server) WaitStop() {
	s.wg.Wait()
}

func (s *Server) Listen() error {
	const op = "server.Listen"
	log := s.log.With("op", op)
	// Add Listen() to WaitGroup
	s.wg.Add(1)
	defer s.wg.Done()

	// Initialize the listener
	listener, err := net.Listen(s.protocol, s.addr)
	if err != nil {
		return err
	}
	var closeOnce sync.Once
	closeListener := func() { closeOnce.Do(func() { listener.Close() }) }
	defer closeListener()

	log.Info("Server started and listening...", "addr", s.addr)

	// Initialize the context
	ctx, cancel := context.WithCancel(context.Background())
	s.exit = cancel

	// We launch a goroutine that will close the listener when the context is canceled
	go func() {
		<-ctx.Done()
		closeListener()
	}()

	for {
		// Trying to accept a new connection
		conn, err := listener.Accept()
		if err != nil {
			switch {
			case errors.Is(err, net.ErrClosed):
				log.Info("Socket is closed, stopped waiting for new connections")
				return nil
			default:
				log.Error("Error accepting connection", err)
				return err
			}
		}

		// Launching the connection handler
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(ctx, conn)
		}()
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	const op = "server.handleConnection"
	log := s.log.With(
		slog.String("op", op),
		slog.Any("addr", conn.RemoteAddr()),
	)
	log.Info("New connection")

	//
	var closeOnce sync.Once
	closeConn := func() { closeOnce.Do(func() { conn.Close() }) }

	defer closeConn()

	// Launching a routine that closes the connection when the context is canceled
	go func() {
		<-ctx.Done()
		closeConn()
	}()

	for {
		// Read the beginning of the packet from the socket
		header := make([]byte, 4)
		if _, err := io.ReadFull(conn, header); err != nil {
			switch {
			case errors.Is(err, io.EOF):
				log.Info("Client closed connection")
			case errors.Is(err, net.ErrClosed):
				log.Info("Connection is closed")
			default:
				log.Error("Error reading header", err)
			}
			return
		}

		// Extract the length of the packet
		pkgLength := binary.BigEndian.Uint32(header)

		// If the packet is incomplete, close the connection
		if pkgLength < 8 {
			log.Error("Incorrect package length", "pkgLength", pkgLength)
			return
		}

		// If the packet is too large, close the connection
		if pkgLength > s.maxPacketSize {
			log.Error("Packet is too big", "pkgLength", pkgLength, "maxPacketSize", s.maxPacketSize)
			return
		}

		// Read the rest of the packet (pkgLength - 4 bytes)
		body := make([]byte, pkgLength-4)
		if _, err := io.ReadFull(conn, body); err != nil {
			switch {
			case errors.Is(err, io.EOF):
				log.Info("Client closed connection")
			case errors.Is(err, net.ErrClosed):
				log.Info("Connection is closed")
			default:
				log.Error("Error reading packet", err)
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
			log.Error("Received packet with unknown ID", "ID", pkg.ID)
			continue
		}

		// Launching a user handler
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			handler(ctx, conn, *pkg)
		}()
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
