// Command client runs the chat client.
//
//	go run ./cmd/client --addr 127.0.0.1:12345 --name ostap --room 1
//
// Once connected:
//
//	/join <room>   join or switch rooms
//	/leave         leave the current room
//	/send <path>   send a file to the room
//	/quit          disconnect
//	<text>         send a chat message
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/0stapyan/go-chat-server/internal/client"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:12345", "server address")
	name := flag.String("name", "", "your name (required)")
	room := flag.String("room", "1", "room to join on connect")
	downloads := flag.String("downloads", "downloads", "directory for received files")
	flag.Parse()

	if *name == "" {
		fmt.Fprintln(os.Stderr, "error: --name is required")
		flag.Usage()
		os.Exit(2)
	}

	c, err := client.Dial(*addr, *name, *room, *downloads)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	if err := c.Run(); err != nil {
		log.Fatal(err)
	}
}
