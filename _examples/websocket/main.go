package main

import (
	"embed"
	"log"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/response"
)

//go:embed public
var publicFiles embed.FS

func echo(socket *response.Socket) {
	defer socket.Close(1000, "bye")

	for {
		msg, err := socket.Read()
		if err != nil {
			log.Printf("read: %v", err)
			return
		}

		log.Printf("received: %s", msg)

		if err := socket.Send("echo: " + msg); err != nil {
			log.Printf("send: %v", err)
			return
		}
	}
}

func main() {
	httpmin.New().
		RouteHandler("/echo", response.WebSocket(echo)).
		ServeEmbedded(publicFiles).
		Run()
}
