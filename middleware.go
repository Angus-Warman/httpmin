package httpmin

import (
	"log"
	"net/http"
	"runtime"
)

func logRequests(logger *log.Logger) func(http.Handler) http.Handler {
	f := func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			logger.Printf("%v %v\n", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}

	return f
}

func recoverPanics(logger *log.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = log.Default()
	}

	f := func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					buf := make([]byte, 4096)
					n := runtime.Stack(buf, false)
					logger.Printf("PANIC: %v\n%s", rec, buf[:n])

					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}

	return f
}
