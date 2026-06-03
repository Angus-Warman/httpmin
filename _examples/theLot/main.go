package main

import (
	"embed"
	"net/http"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/handler"
	"github.com/Angus-Warman/httpmin/middleware"
)

func ping(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("pong"))
}

func hello(w http.ResponseWriter, r *http.Request) (any, error) {
	name := r.URL.Query().Get("name")

	if name == "" {
		name = "World"
	}

	return map[string]string{
		"Name": name,
	}, nil
}

func myCustomMiddleware() func(http.Handler) http.Handler {
	f := func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			// Do something here
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}

	return f
}

//go:embed templates
var templatesFS embed.FS

func main() {
	c := httpmin.Setup() // Modifications can be chained

	c.OnPort("8081") // Port used comes from: env variables, .env file, this function, "8080" (in that order)
	c.Route("/ping", ping)
	c.RouteHandler("/hello", handler.Template(templatesFS, "hello.tmpl", hello))
	c.RouteHandler("/stats", handler.Stats())
	c.ServeFolder("public") // Not embedded, add any file to folder and load the page
	c.Use(middleware.Cors())
	c.Use(myCustomMiddleware())
	c.PublicIP() // Listen on "0.0.0.0"

	c.Run()
}
