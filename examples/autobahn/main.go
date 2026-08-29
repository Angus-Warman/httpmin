package main

import (
	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/response"
)

func echo(ws *response.WebSocketConnection) {
	defer ws.Close()

	for {
		msg, err := ws.Read()

		if err != nil {
			return
		}

		ws.Send(msg)
	}
}

func main() {
	httpmin.New().
		OnPort("9001").
		RouteHandler("/echo", response.WebSocket(echo)).
		Run()
}
