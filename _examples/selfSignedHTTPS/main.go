package main

import (
	"net/http"

	"github.com/Angus-Warman/httpmin"
)

func helloWorld(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello World"))
}

func main() {
	httpmin.New().Route("/", helloWorld).UseSelfSignedHTTPSFromFolder("tls").Run()
}
