package server

import (
	"fmt"
	"log"

	"github.com/0stapyan/go-chat-server/internal/protocol"
)

// hub owns all shared chat state: which clients exist and which room each is in.
//
// Instead of protecting that state with a mutex, the hub runs on a single
// goroutine and receives every change over channels. Only hub.run() ever reads
// or writes the maps, so there is no shared-memory race by construction. This is
// the standard Go approach: "share memory by communicating".
type hub struct {
	register   chan *client
	unregister chan *client
	commands   chan command

	// Owned by run() only.
	clients     map[*client]struct{}
	rooms       map[string]map[*client]struct{}
	transferSeq int
}

// command is a request from a client's reader goroutine to the hub.
type command struct {
	from  *client
	frame protocol.Frame
}

func newHub() *hub {
	return &hub{
		register:   make(chan *client),
		unregister: make(chan *client),
		commands:   make(chan command, 256),
		clients:    make(map[*client]struct{}),
		rooms:      make(map[string]map[*client]struct{}),
	}
}

func (h *hub) run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = struct{}{}
			log.Printf("client %q connected", c.name)

		case c := <-h.unregister:
			h.removeFromRoom(c)
			delete(h.clients, c)
			c.close()
			log.Printf("client %q disconnected", c.name)

		case cmd := <-h.commands:
			h.handle(cmd)
		}
	}
}

func (h *hub) handle(cmd command) {
	c, frame := cmd.from, cmd.frame
	switch frame.Type {
	case protocol.Join:
		h.joinRoom(c, string(frame.Payload))
	case protocol.Leave:
		h.leaveRoom(c)
	case protocol.Chat:
		h.broadcastChat(c, string(frame.Payload))
	case protocol.FileOffer:
		h.handleFileOffer(c, frame.Payload)
	case protocol.FileChunk:
		h.handleFileChunk(c, frame.Payload)
	default:
		c.send(protocol.Error, []byte("unsupported command"))
	}
}

func (h *hub) joinRoom(c *client, room string) {
	if room == "" {
		c.send(protocol.Error, []byte("room name cannot be empty"))
		return
	}
	if c.room == room {
		c.send(protocol.Info, []byte("you are already in room "+room))
		return
	}
	h.removeFromRoom(c)

	c.room = room
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[*client]struct{})
	}
	h.rooms[room][c] = struct{}{}

	c.send(protocol.Info, []byte("joined room "+room))
	h.announce(room, c, fmt.Sprintf("CLIENT %s JOINED ROOM %s", c.name, room))
	log.Printf("%q joined room %q", c.name, room)
}

func (h *hub) leaveRoom(c *client) {
	if c.room == "" {
		c.send(protocol.Error, []byte("you are not in a room"))
		return
	}
	room := c.room
	h.removeFromRoom(c)
	c.send(protocol.Info, []byte("left room "+room))
}

// removeFromRoom takes the client out of its current room and tells the others.
func (h *hub) removeFromRoom(c *client) {
	if c.room == "" {
		return
	}
	room := c.room
	if members := h.rooms[room]; members != nil {
		delete(members, c)
		if len(members) == 0 {
			delete(h.rooms, room)
		}
	}
	h.announce(room, c, fmt.Sprintf("CLIENT %s LEFT ROOM %s", c.name, room))
	c.room = ""
}

func (h *hub) broadcastChat(c *client, text string) {
	if c.room == "" {
		c.send(protocol.Error, []byte("join a room before sending messages"))
		return
	}
	payload := []byte(c.name + "\n" + text)
	for member := range h.rooms[c.room] {
		if member != c {
			member.send(protocol.ChatFrom, payload)
		}
	}
}

// announce sends an informational notice to everyone in a room except origin.
func (h *hub) announce(room string, origin *client, text string) {
	for member := range h.rooms[room] {
		if member != origin {
			member.send(protocol.Info, []byte(text))
		}
	}
}
