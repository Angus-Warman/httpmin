package main

import (
	"embed"
	"net/http"
	"os"

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
var templatesFS embed.FS

func main() {
	os.Setenv("PASSWORD", "12345")

	c := httpmin.New().
		OnPort("8081"). // Port used comes from: env variables, .env file, this function, "8080" (in that order).
		Route("/ping", ping).
		RouteHandler("/hello", handler.Template(templatesFS, "hello.tmpl", hello)).
		RouteHandler("/stats", handler.Stats()).
		RouteHandler("/secret", middleware.BasicAuth()(secret())).
		ServeFolder("public"). // Not embedded, add any file to folder and load the page
		Use(middleware.Cors()).
		Use(myCustomMiddleware()).
		PublicIP() // Listen on "0.0.0.0"

	c.Run()
}
