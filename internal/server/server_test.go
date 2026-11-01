package server

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/0stapyan/go-chat-server/internal/protocol"
)

// startTestServer launches a server on a random free port and returns its address.
func startTestServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{hub: newHub()}
	go s.hub.run()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

// testConn is a client connection for use in tests.
type testConn struct {
	conn net.Conn
	rd   *bufio.Reader
}

func dial(t *testing.T, addr, name, room string) *testConn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	tc := &testConn{conn: conn, rd: bufio.NewReader(conn)}
	if err := protocol.Write(conn, protocol.Join, []byte(name+"\n"+room)); err != nil {
		t.Fatal(err)
	}
	return tc
}

// readUntil returns the first frame of the given type, skipping others, or fails.
func (tc *testConn) readUntil(t *testing.T, want protocol.MessageType) protocol.Frame {
	t.Helper()
	tc.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		frame, err := protocol.Read(tc.rd)
		if err != nil {
			t.Fatalf("waiting for type %d: %v", want, err)
		}
		if frame.Type == want {
			return frame
		}
	}
}

func TestChatBroadcast(t *testing.T) {
	addr := startTestServer(t)
	alice := dial(t, addr, "alice", "room1")
	bob := dial(t, addr, "bob", "room1")

	// Both get a "joined room" Info; drain alice's so ordering is clean.
	alice.readUntil(t, protocol.Info)
	bob.readUntil(t, protocol.Info)

	if err := protocol.Write(alice.conn, protocol.Chat, []byte("hello bob")); err != nil {
		t.Fatal(err)
	}

	frame := bob.readUntil(t, protocol.ChatFrom)
	sender, text, _ := strings.Cut(string(frame.Payload), "\n")
	if sender != "alice" || text != "hello bob" {
		t.Fatalf("got %q from %q, want \"hello bob\" from \"alice\"", text, sender)
	}
}

func TestRoomsAreIsolated(t *testing.T) {
	addr := startTestServer(t)
	alice := dial(t, addr, "alice", "room1")
	bob := dial(t, addr, "bob", "room2")
	alice.readUntil(t, protocol.Info)
	bob.readUntil(t, protocol.Info)

	protocol.Write(alice.conn, protocol.Chat, []byte("only room1"))

	// Bob is in room2 and must not receive it. Expect a read timeout.
	bob.conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	if _, err := protocol.Read(bob.rd); err == nil {
		t.Fatal("bob in room2 received a message meant for room1")
	}
}

func TestFileTransfer(t *testing.T) {
	addr := startTestServer(t)
	alice := dial(t, addr, "alice", "room1")
	bob := dial(t, addr, "bob", "room1")
	alice.readUntil(t, protocol.Info)
	bob.readUntil(t, protocol.Info)

	protocol.Write(alice.conn, protocol.FileOffer, []byte("notes.txt\n5"))
	offer := bob.readUntil(t, protocol.IncomingOffer)
	if !strings.Contains(string(offer.Payload), "notes.txt") {
		t.Fatalf("offer did not mention the file name: %q", offer.Payload)
	}

	protocol.Write(alice.conn, protocol.FileChunk, []byte("notes.txt\nhello"))
	file := bob.readUntil(t, protocol.IncomingFile)
	name, data, _ := strings.Cut(string(file.Payload), "\n")
	if name != "notes.txt" || data != "hello" {
		t.Fatalf("got file %q with %q, want notes.txt with hello", name, data)
	}
}

func TestSwitchRoom(t *testing.T) {
	addr := startTestServer(t)
	alice := dial(t, addr, "alice", "room1")
	alice.readUntil(t, protocol.Info) // joined room1

	protocol.Write(alice.conn, protocol.Join, []byte("room2"))
	frame := alice.readUntil(t, protocol.Info)
	if !strings.Contains(string(frame.Payload), "room2") {
		t.Fatalf("expected confirmation of room2, got %q", frame.Payload)
	}
}
