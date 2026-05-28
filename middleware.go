package httpmin

import (
	"log"
	"net/http"
)

func requestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	f := func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			logger.Printf("%v %v\n", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}

	return f
}
