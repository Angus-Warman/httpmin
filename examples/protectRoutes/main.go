package main

import (
	"embed"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/middleware"
)

func isValid(username, password string) bool {
	return username != "" && password != ""
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	username := r.Form.Get("username")
	password := r.Form.Get("password")

	if !isValid(username, password) {
		http.Error(w, "Invalid credentials", 403)
		return
	}

	err := middleware.Authorize(username, w)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	returnTo := r.Form.Get("returnTo")

	if isSafeRedirect(returnTo) {
		http.Redirect(w, r, returnTo, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func isSafeRedirect(s string) bool {
	return strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//")
}

//go:embed all:public
var publicFiles embed.FS

func main() {
	os.Setenv("JWT_SECRET", fmt.Sprint(rand.Int64()))

	c := httpmin.New()

	c.Route("POST /login", loginHandler)
	c.ServeEmbedded(publicFiles)

	c.Use(middleware.ProtectRoutes(
		middleware.ProtectRoutesConfig{
			SecuredRoutes: "/secret",
			Redirect:      "/login",
		}))

	c.Run()
}
