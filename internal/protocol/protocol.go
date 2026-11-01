// Package protocol defines the wire format shared by the chat server and client.
//
// Every message is a single frame:
//
//	+--------+-----------------+-------------------------+
//	| type   | length (4 bytes)|  payload (length bytes) |
//	| 1 byte | big-endian uint |                         |
//	+--------+-----------------+-------------------------+
//
// The one-byte type tells the receiver how to read the payload, and the length
// prefix means text messages and raw file bytes travel over the same connection
// without one being mistaken for the other. This is the core idea the chat
// relies on: TCP is a byte stream with no message boundaries, so the boundaries
// are added here.
package protocol

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MessageType identifies what a frame carries.
type MessageType byte

const (
	// Client -> server
	Join       MessageType = 1 // payload: room name; join or switch rooms
	Chat       MessageType = 2 // payload: a chat line
	FileOffer  MessageType = 3 // payload: "<name>\n<size>"; propose sending a file
	FileAccept MessageType = 4 // payload: transfer id; this client will receive
	FileReject MessageType = 5 // payload: transfer id; this client declines
	FileChunk  MessageType = 6 // payload: "<transfer id>\n" then raw bytes
	Leave      MessageType = 7 // payload: empty; leave the current room
	Quit       MessageType = 8 // payload: empty; disconnect

	// Server -> client
	Info          MessageType = 20 // payload: a human-readable notice
	ChatFrom      MessageType = 21 // payload: "<sender>\n<text>"
	Error         MessageType = 22 // payload: an error message
	IncomingOffer MessageType = 23 // payload: "<id>\n<sender>\n<name>\n<size>"
	IncomingFile  MessageType = 24 // payload: "<name>\n" then raw bytes
	TransferDone  MessageType = 25 // payload: a notice that a transfer finished
)

// MaxPayload caps a single frame so a bad or hostile peer cannot make the
// receiver allocate unbounded memory. File transfers are split into chunks
// below this size.
const MaxPayload = 1 << 20 // 1 MiB

// ErrPayloadTooLarge is returned when a frame claims a payload above MaxPayload.
var ErrPayloadTooLarge = errors.New("protocol: payload exceeds maximum size")

// Frame is one decoded message.
type Frame struct {
	Type    MessageType
	Payload []byte
}

// Write encodes a frame to w. It is safe for one writer at a time; callers that
// share a connection across goroutines must serialise their writes.
func Write(w io.Writer, t MessageType, payload []byte) error {
	if len(payload) > MaxPayload {
		return ErrPayloadTooLarge
	}
	var header [5]byte
	header[0] = byte(t)
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// Read decodes the next frame from r. It returns io.EOF when the peer closes
// the connection cleanly between frames.
func Read(r *bufio.Reader) (Frame, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, err
	}
	length := binary.BigEndian.Uint32(header[1:])
	if length > MaxPayload {
		return Frame{}, fmt.Errorf("%w: %d bytes", ErrPayloadTooLarge, length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Frame{}, err
	}
	return Frame{Type: MessageType(header[0]), Payload: payload}, nil
}
