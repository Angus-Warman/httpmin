package main

import (
	"embed"
	"net/http"
	"os"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/handler"
	"github.com/Angus-Warman/httpmin/middleware"
	"github.com/Angus-Warman/httpmin/response"
)

func ping(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("pong"))
}

func hello(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")

	if name == "" {
		name = "World"
	}

	data := map[string]string{
		"Name": name,
	}

	response.Template(tmpls, w, "hello", data)
}

func secret() http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret"))
	}

	return http.HandlerFunc(f)
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
var templateFiles embed.FS
var tmpls = response.PrepareTemplate(templateFiles)

func main() {
	os.Setenv("PASSWORD", "12345")

	c := httpmin.New().
		OnPort("8081"). // Port used comes from: env variables, .env file, this function, "8080" (in that order).
		Route("/ping", ping).
		Route("/hello", hello).
		RouteHandler("/stats", handler.Stats()).
		RouteHandler("/secret", middleware.BasicAuth()(secret())).
		ServeFolder("public"). // Not embedded, add any file to folder and load the page
		Use(middleware.Cors()).
		Use(myCustomMiddleware()).
		PublicIP() // Listen on "0.0.0.0"

	c.Run()
}
