package middleware

import (
	"log"
	"net/http"
	"runtime"
)

func RecoverPanics(logger *log.Logger) func(http.Handler) http.Handler {
	// This one is an edge case
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
