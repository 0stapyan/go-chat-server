// Package client implements the chat client: it connects to the server, reads
// user commands from stdin, and prints messages and files coming from the room.
package client

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/0stapyan/go-chat-server/internal/protocol"
)

// Client holds a single connection to the server.
type Client struct {
	name      string
	conn      net.Conn
	rd        *bufio.Reader
	writeMu   sync.Mutex // serialises writes from the stdin loop and auto-replies
	downloads string     // directory where received files are saved
}

// Dial connects to addr and performs the "<name>\n<room>" handshake.
func Dial(addr, name, room, downloads string) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	c := &Client{
		name:      name,
		conn:      conn,
		rd:        bufio.NewReader(conn),
		downloads: downloads,
	}
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		return nil, err
	}
	if err := c.write(protocol.Join, []byte(name+"\n"+room)); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) write(t protocol.MessageType, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return protocol.Write(c.conn, t, payload)
}

// Run starts the reader goroutine and processes stdin until the user quits or
// the connection drops.
func (c *Client) Run() error {
	go c.readLoop()
	return c.inputLoop()
}

// readLoop prints everything the server sends and saves incoming files.
func (c *Client) readLoop() {
	for {
		frame, err := protocol.Read(c.rd)
		if err != nil {
			fmt.Println("\n[disconnected from server]")
			os.Exit(0)
		}
		switch frame.Type {
		case protocol.Info:
			fmt.Printf("\r* %s\n> ", frame.Payload)
		case protocol.Error:
			fmt.Printf("\r! %s\n> ", frame.Payload)
		case protocol.ChatFrom:
			sender, text, _ := strings.Cut(string(frame.Payload), "\n")
			fmt.Printf("\r%s: %s\n> ", sender, text)
		case protocol.IncomingOffer:
			c.printOffer(frame.Payload)
		case protocol.IncomingFile:
			c.saveFile(frame.Payload)
		}
	}
}

func (c *Client) printOffer(payload []byte) {
	fields := strings.SplitN(string(payload), "\n", 4)
	if len(fields) == 4 {
		fmt.Printf("\r* CLIENT %s wants to send %s which size is %s bytes\n> ",
			fields[1], fields[2], fields[3])
	}
}

// saveFile writes an incoming file ("<name>\n" then raw bytes) to the downloads
// directory, guarding against path traversal in the supplied name.
func (c *Client) saveFile(payload []byte) {
	name, data, ok := strings.Cut(string(payload), "\n")
	if !ok {
		return
	}
	safe := filepath.Base(name)
	path := filepath.Join(c.downloads, safe)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		fmt.Printf("\r! could not save %s: %v\n> ", safe, err)
		return
	}
	fmt.Printf("\rFile %s has been downloaded\n> ", safe)
}

// inputLoop reads commands from stdin. Supported commands:
//
//	/join <room>      join or switch to a room
//	/leave            leave the current room
//	/send <path>      send a file to everyone in the room
//	/quit             disconnect
//	<anything else>   a chat message
func (c *Client) inputLoop() error {
	in := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for in.Scan() {
		line := strings.TrimRight(in.Text(), "\r\n")
		if line == "" {
			fmt.Print("> ")
			continue
		}

		switch {
		case strings.HasPrefix(line, "/join "):
			room := strings.TrimSpace(line[len("/join "):])
			_ = c.write(protocol.Join, []byte(room))
		case line == "/leave":
			_ = c.write(protocol.Leave, nil)
		case strings.HasPrefix(line, "/send "):
			path := strings.TrimSpace(line[len("/send "):])
			c.sendFile(path)
		case line == "/quit":
			_ = c.write(protocol.Quit, nil)
			return nil
		default:
			_ = c.write(protocol.Chat, []byte(line))
		}
		fmt.Print("> ")
	}
	return in.Err()
}

// sendFile offers a file to the room and then streams it in chunks.
func (c *Client) sendFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("! cannot read %s: %v\n", path, err)
		return
	}
	name := filepath.Base(path)

	if err := c.write(protocol.FileOffer, []byte(name+"\n"+strconv.Itoa(len(data)))); err != nil {
		fmt.Printf("! offer failed: %v\n", err)
		return
	}

	// Send the whole file as one IncomingFile payload per chunk. Each chunk is
	// "<name>\n" + bytes, kept under the protocol's frame limit.
	const chunkSize = protocol.MaxPayload - 256
	header := name + "\n"
	for off := 0; off < len(data); off += chunkSize {
		end := off + chunkSize
		if end > len(data) {
			end = len(data)
		}
		frame := append([]byte(header), data[off:end]...)
		if err := c.write(protocol.FileChunk, frame); err != nil {
			fmt.Printf("! send failed: %v\n", err)
			return
		}
	}
	fmt.Printf("File %s sent (%d bytes)\n", name, len(data))
}
