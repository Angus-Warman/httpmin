package main

import (
	"net/http"

	"github.com/Angus-Warman/httpmin"
)

func whoops(w http.ResponseWriter, r *http.Request) {
	data := make([]byte, 5)
	value := data[10]
	_ = value
}

func main() {
	httpmin.Setup().Route("/whoops", whoops).Run()
}
