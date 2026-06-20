package response

import (
	"fmt"
	"net/http"
)

// Turns a long-running operation into a server-sent event stream
func Stream(operation func(r *http.Request, notify func(event string)) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)

		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// nginx
		w.Header().Set("X-Accel-Buffering", "no")

		notify := func(event string) {
			if r.Context().Err() != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}

		err := operation(r, notify)

		if err != nil && r.Context().Err() == nil {
			notify(err.Error())
		}
	})
}
