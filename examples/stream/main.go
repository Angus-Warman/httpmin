package main

import (
	"log"
	"net/http"
	"time"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/response"
)

func timeStream(w http.ResponseWriter, r *http.Request) {
	stream := response.Stream(w, r)

	for {
		if stream.Closed() {
			log.Println("stream closed")
			return
		}

		now := time.Now()

		stream.Send(now.String())

		time.Sleep(1 * time.Second)
	}
}

func main() {
	httpmin.New().Route("/time", timeStream).Run()
}
