package main

import (
	"log"
	"net/http"
	"time"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/response"
)

func timeStream(r *http.Request, notify func(string)) error {
	for {
		err := r.Context().Err()

		if err != nil {
			log.Println(err)
			return err
		}

		now := time.Now()

		log.Println(now.String())
		notify(now.String())

		time.Sleep(1 * time.Second)
	}
}

func main() {
	httpmin.New().RouteHandler("/time", response.Stream(timeStream)).Run()
}
