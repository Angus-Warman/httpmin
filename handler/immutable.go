package handler

import (
	"net/http"
	"time"
)

// If the result of "next" never changes while the server is running,
// Immutable will correctly serve 304 status responses
func Immutable(next http.Handler) http.Handler {
	startTime := time.Now().UTC().Round(time.Second)
	startTimeString := startTime.Format(http.TimeFormat)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isNotModifiedSince(r, startTime) {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Last-Modified", startTimeString)
		next.ServeHTTP(w, r)
	})
}

func isNotModifiedSince(r *http.Request, startTime time.Time) bool {
	ifModifiedSinceStr := r.Header.Get("If-Modified-Since")

	if ifModifiedSinceStr == "" {
		return false
	}

	ifModifiedSince, err := http.ParseTime(ifModifiedSinceStr)

	if err != nil {
		return false
	}

	stale := ifModifiedSince.Before(startTime)

	return !stale
}
