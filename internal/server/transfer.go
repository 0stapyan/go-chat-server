package server

import (
	"bytes"
	"fmt"
	"github.com/0stapyan/go-chat-server/internal/protocol"
	"strconv"
)

// File sharing works like this:
//
//  1. The sender issues a FileOffer with a file name and size.
//  2. The hub gives the transfer an id and sends an IncomingOffer to every other
//     member of the room, who can accept or decline.
//  3. The sender streams the file as FileChunk frames. The hub forwards each
//     chunk only to the members who accepted, as IncomingFile frames.
//
// Accept and reject are handled on the client side in this simple version: the
// server forwards the file to every other member and each client decides whether
// to save it. The offer carries the size so a client can be asked first.

// handleFileOffer assigns an id to a new transfer and notifies the room.
func (h *hub) handleFileOffer(c *client, payload []byte) {
	if c.room == "" {
		c.send(protocol.Error, []byte("join a room before sending files"))
		return
	}
	name, size, ok := parseOffer(payload)
	if !ok {
		c.send(protocol.Error, []byte("malformed file offer"))
		return
	}

	id := h.nextTransferID()
	notice := fmt.Sprintf("%d\n%s\n%s\n%d", id, c.name, name, size)
	recipients := 0
	for member := range h.rooms[c.room] {
		if member != c {
			member.send(protocol.IncomingOffer, []byte(notice))
			recipients++
		}
	}
	c.send(protocol.Info, []byte(fmt.Sprintf(
		"CLIENT %s sends a file %s... (%d recipient(s))", c.name, name, recipients)))
}

// handleFileChunk forwards one chunk of a file to the other room members.
func (h *hub) handleFileChunk(c *client, payload []byte) {
	if c.room == "" {
		return
	}
	for member := range h.rooms[c.room] {
		if member != c {
			member.send(protocol.IncomingFile, payload)
		}
	}
}

func (h *hub) nextTransferID() int {
	h.transferSeq++
	return h.transferSeq
}

// parseOffer reads "<name>\n<size>" from a FileOffer payload.
func parseOffer(payload []byte) (name string, size int64, ok bool) {
	parts := bytes.SplitN(payload, []byte{'\n'}, 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	size, err := strconv.ParseInt(string(parts[1]), 10, 64)
	if err != nil {
		return "", 0, false
	}
	return string(parts[0]), size, true
}
