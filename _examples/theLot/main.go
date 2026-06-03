package main

import (
	"net/http"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/handler"
	"github.com/Angus-Warman/httpmin/middleware"
)

func helloWorld(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello World"))
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

func main() {
	c := httpmin.Setup() // Modifications can be chained

	c.OnPort("8081") // Port used comes from: env variables, .env file, this function, "8080" (in that order)
	c.Route("/hello", helloWorld)
	c.RouteHandler("/stats", handler.Stats())
	c.ServeFolder("public") // Not embedded, add any file to folder and load the page
	c.Use(middleware.Cors())
	c.Use(myCustomMiddleware())
	c.PublicIP() // Listen on "0.0.0.0"
	c.UseSelfSignedHTTPS()

	c.Run()
}
