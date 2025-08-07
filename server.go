package scutium

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/vscerlet/scutium/lib"
)

type HandlerFunc func(ctx context.Context, conn net.Conn, pkg BasicPacket) error

type Server struct {
	addr          string
	protocol      string
	maxPacketSize uint32
	handlers      sync.Map
	exit          context.CancelFunc
	wg            sync.WaitGroup
	log           *slog.Logger
	clientTimeout time.Duration
	m             sync.Mutex
}

func NewServer(addr string, protocol string) *Server {
	return &Server{
		addr:          addr,
		protocol:      protocol,
		maxPacketSize: 1024 * 1024, // 1 MB
		log:           slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
		clientTimeout: 30 * time.Second,
	}
}

func (s *Server) SetLogger(log *slog.Logger) {
	s.m.Lock()
	defer s.m.Unlock()
	s.log = log
}

func (s *Server) SetMaxPacketSize(size uint32) {
	s.m.Lock()
	defer s.m.Unlock()
	s.maxPacketSize = size
}

func (s *Server) SetClientTimeout(timeout time.Duration) {
	s.m.Lock()
	defer s.m.Unlock()
	s.clientTimeout = timeout
}

func (s *Server) On(pkgType uint32, handler HandlerFunc) {
	s.handlers.Store(pkgType, handler)
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
	op := lib.GetCurrentFuncName()
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
				log.Error("Error accepting connection", slog.Any("err", err))
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
	op := lib.GetCurrentFuncName()
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
		conn.SetReadDeadline(time.Now().Add(s.clientTimeout))
		header := make([]byte, 4)
		if _, err := io.ReadFull(conn, header); err != nil {
			switch {
			case errors.Is(err, io.EOF):
				log.Info("Client closed connection")
			case errors.Is(err, net.ErrClosed):
				log.Info("Connection is closed")
			case errors.Is(err, os.ErrDeadlineExceeded):
				log.Info("Connection terminated due to client inactivity", "clientTimeout", s.clientTimeout)
			default:
				log.Error("Error reading header", slog.Any("err", err))
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
		conn.SetReadDeadline(time.Now().Add(s.clientTimeout))
		body := make([]byte, pkgLength-4)
		if _, err := io.ReadFull(conn, body); err != nil {
			switch {
			case errors.Is(err, io.EOF):
				log.Info("Client closed connection")
			case errors.Is(err, net.ErrClosed):
				log.Info("Connection is closed")
			case errors.Is(err, os.ErrDeadlineExceeded):
				log.Info("Connection terminated due to client inactivity", "clientTimeout", s.clientTimeout)
			default:
				log.Error("Error reading packet", slog.Any("err", err))
			}
			return
		}

		// Collect the complete package
		packet := append(header, body...)

		// Parsing packet fields
		pkg := parseBasicPacket(packet)

		// Get a handler for the package
		handler, ok := s.handlers.Load(pkg.ID)
		if !ok {
			log.Error("Received packet with unknown ID", "ID", pkg.ID)
			continue
		}
		// Get value from map; check if this is function; cast it to HandlerFunc
		var handlerFunc HandlerFunc
		if h, isFunc := handler.(HandlerFunc); isFunc {
			handlerFunc = h
		} else {
			log.Error("handler is not a function")
			return
		}

		// Launching a user handler
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			handlerFunc(ctx, conn, *pkg)
		}()
	}
}

func SendPkg(conn net.Conn, pkgID uint32, payload []byte) (int, error) {
	op := lib.GetCurrentFuncName()
	totalLength := uint32(8 + len(payload))
	buf := make([]byte, totalLength)

	binary.BigEndian.PutUint32(buf[0:4], totalLength)
	binary.BigEndian.PutUint32(buf[4:8], pkgID)
	copy(buf[8:], payload)

	n, err := conn.Write(buf)
	if err != nil {
		return n, fmt.Errorf("%s: %w", op, err)
	}
	return n, nil
}
