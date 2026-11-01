// Command server runs the chat server.
//
//	go run ./cmd/server --addr :12345
package main

import (
	"flag"
	"log"

	"github.com/0stapyan/go-chat-server/internal/server"
)

func main() {
	addr := flag.String("addr", ":12345", "address to listen on")
	flag.Parse()

	if err := server.New(*addr).ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
