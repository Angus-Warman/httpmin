package main

import (
	"net/http"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/handler"
)

func main() {
	store := &UserStore{}
	renderer := NewUserRenderer()

	c := httpmin.New().
		Route("GET /", func(w http.ResponseWriter, r *http.Request) {
			renderer.RenderIndex(w)
		})

	handler.Builder(c.Mux, "/api/users", store, renderer)

	c.Run()
}
