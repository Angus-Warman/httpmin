package middleware

import (
	"fmt"
	"net/http"
	"os"
)

var BasicAuthSettings = struct {
	// "USERNAME"
	UsernameEnvKey string

	// "PASSWORD"
	PasswordEnvKey string
}{
	UsernameEnvKey: "USERNAME",
	PasswordEnvKey: "PASSWORD",
}

func BasicAuth() func(http.Handler) http.Handler {
	username := os.Getenv(BasicAuthSettings.UsernameEnvKey) // Can be an empty string

	password, ok := os.LookupEnv(BasicAuthSettings.PasswordEnvKey)

	if !ok {
		err := fmt.Errorf("%v environment variable not set", BasicAuthSettings.PasswordEnvKey)
		panic(err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()

			if !ok || user != username || pass != password {
				w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
