package server

import (
	"bufio"
	"net"
	"sync"

	"github.com/0stapyan/go-chat-server/internal/protocol"
)

// client is one connected user, as seen by the hub.
//
// Each client has a dedicated goroutine that owns all writes to its socket, fed
// by the buffered outbound channel. Any other goroutine (the hub, another
// client's reader) sends to a client only through send(), never by touching the
// connection directly, so writes never race.
type client struct {
	name string
	room string
	conn net.Conn
	rd   *bufio.Reader

	outbound chan protocol.Frame
	once     sync.Once
	closed   chan struct{}
}

func newClient(name string, conn net.Conn) *client {
	return &client{
		name:     name,
		conn:     conn,
		rd:       bufio.NewReader(conn),
		outbound: make(chan protocol.Frame, 64),
		closed:   make(chan struct{}),
	}
}

// send queues a frame for delivery. If the client's buffer is full (a slow or
// stuck receiver) or the client is closing, the frame is dropped rather than
// blocking the hub or the sender.
func (c *client) send(t protocol.MessageType, payload []byte) {
	select {
	case c.outbound <- protocol.Frame{Type: t, Payload: payload}:
	case <-c.closed:
	default:
	}
}

// writeLoop is the only goroutine that writes to the connection.
func (c *client) writeLoop() {
	for {
		select {
		case frame := <-c.outbound:
			if err := protocol.Write(c.conn, frame.Type, frame.Payload); err != nil {
				return
			}
		case <-c.closed:
			return
		}
	}
}

// close stops the client's goroutines and connection exactly once.
func (c *client) close() {
	c.once.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
	})
}
