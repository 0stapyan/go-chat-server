// Package server implements the multi-room chat server over TCP.
//
// Concurrency model:
//   - One hub goroutine owns all room/client state (see hub.go).
//   - Each connection gets one reader goroutine (readLoop) and one writer
//     goroutine (writeLoop). Readers send commands to the hub over a channel;
//     the hub and other goroutines reach a client only through its send() queue.
//
// This keeps shared state on a single goroutine, so the race detector stays
// quiet, and a slow client cannot block anyone else.
package server

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"strings"

	"github.com/0stapyan/go-chat-server/internal/protocol"
)

// Server accepts connections and runs the hub.
type Server struct {
	addr string
	hub  *hub
}

func New(addr string) *Server {
	return &Server{addr: addr, hub: newHub()}
}

// ListenAndServe starts the hub and the accept loop. It blocks until the
// listener fails or is closed.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	go s.hub.run()
	log.Printf("chat server listening on %s", s.addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.serve(conn)
	}
}

// serve performs the handshake and then runs the client's read loop.
//
// Handshake: the first frame must be a Join whose payload is "<name>\n<room>".
// The name identifies the client; the room is the one it starts in.
func (s *Server) serve(conn net.Conn) {
	rd := bufio.NewReader(conn)

	frame, err := protocol.Read(rd)
	if err != nil {
		_ = conn.Close()
		return
	}
	if frame.Type != protocol.Join {
		_ = protocol.Write(conn, protocol.Error, []byte("expected JOIN handshake"))
		_ = conn.Close()
		return
	}

	name, room, ok := strings.Cut(string(frame.Payload), "\n")
	name = strings.TrimSpace(name)
	room = strings.TrimSpace(room)
	if !ok || name == "" || room == "" {
		_ = protocol.Write(conn, protocol.Error, []byte("handshake must be <name>\\n<room>"))
		_ = conn.Close()
		return
	}

	c := newClient(name, conn)
	c.rd = rd
	go c.writeLoop()

	s.hub.register <- c
	s.hub.commands <- command{from: c, frame: protocol.Frame{Type: protocol.Join, Payload: []byte(room)}}

	s.readLoop(c)
}

// readLoop reads frames from a client until it quits or disconnects, forwarding
// each one to the hub. When it returns, the client is unregistered.
func (s *Server) readLoop(c *client) {
	defer func() { s.hub.unregister <- c }()

	for {
		frame, err := protocol.Read(c.rd)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("read from %q: %v", c.name, err)
			}
			return
		}
		if frame.Type == protocol.Quit {
			return
		}
		s.hub.commands <- command{from: c, frame: frame}
	}
}
