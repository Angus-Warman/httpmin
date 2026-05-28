package main

import (
	"embed"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/middleware"
)

func isValid(username, password string) bool {
	return true
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	username := r.PostForm.Get("username")
	password := r.PostForm.Get("password")

	if !isValid(username, password) {
		w.WriteHeader(403)
		return
	}

	err := middleware.Authorize(username, w)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.WriteHeader(200)
}

//go:embed all:public
var publicFiles embed.FS

func main() {
	os.Setenv("JWT_SECRET", fmt.Sprint(rand.Int64()))

	c := httpmin.Setup()

	c.Route("POST /login", loginHandler)
	c.ServeEmbedded(publicFiles)

	c.Use(middleware.ProtectRoutes(
		middleware.ProtectRoutesConfig{
			SecuredRoutes: "/secret",
			Redirect:      "/login",
		}))

	c.Run()
}
