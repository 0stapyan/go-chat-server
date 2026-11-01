package protocol

import (
	"bufio"
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	cases := []struct {
		typ     MessageType
		payload []byte
	}{
		{Chat, []byte("hello room")},
		{Join, []byte("room-1")},
		{Leave, nil},
		{FileChunk, bytes.Repeat([]byte{0xAB}, 4096)},
	}

	var buf bytes.Buffer
	for _, c := range cases {
		if err := Write(&buf, c.typ, c.payload); err != nil {
			t.Fatalf("Write(%d): %v", c.typ, err)
		}
	}

	r := bufio.NewReader(&buf)
	for _, c := range cases {
		frame, err := Read(r)
		if err != nil {
			t.Fatalf("Read(%d): %v", c.typ, err)
		}
		if frame.Type != c.typ {
			t.Errorf("type = %d, want %d", frame.Type, c.typ)
		}
		if !bytes.Equal(frame.Payload, c.payload) {
			t.Errorf("payload mismatch for type %d", c.typ)
		}
	}
}

func TestPayloadTooLargeOnWrite(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, Chat, make([]byte, MaxPayload+1)); err != ErrPayloadTooLarge {
		t.Fatalf("err = %v, want ErrPayloadTooLarge", err)
	}
}

func TestPayloadTooLargeOnRead(t *testing.T) {
	// A hand-built header claiming a payload above the cap must be rejected
	// without allocating it.
	var buf bytes.Buffer
	buf.WriteByte(byte(Chat))
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // length = 4 GiB
	if _, err := Read(bufio.NewReader(&buf)); err == nil {
		t.Fatal("expected error for oversized frame")
	}
}
