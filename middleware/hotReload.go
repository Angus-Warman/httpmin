package middleware

import (
	"bytes"
	"net/http"
	"strings"
	"time"

	"github.com/Angus-Warman/httpmin/response"
)

func HotReload() func(http.Handler) http.Handler {
	f := func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/__ping" {
				response.Stream(pingLoop).ServeHTTP(w, r)
				return
			}

			if !shouldAddHotReloadScript(r) {
				next.ServeHTTP(w, r)
				return
			}

			w2 := &responseReplayer{ResponseWriter: w, status: 200}
			next.ServeHTTP(w2, r)

			body := w2.buf.Bytes()

			isHTML := strings.Contains(w2.Header().Get("Content-Type"), "text/html")

			if isHTML {
				lower := strings.ToLower(string(body))
				bodyTagIdx := strings.LastIndex(lower, "</body>")

				if bodyTagIdx != -1 {
					body = append(body[:bodyTagIdx], append([]byte(hotReloadScript), body[bodyTagIdx:]...)...)
				} else {
					body = append(body, hotReloadScript...)
				}

				w.Header().Del("Content-Length")
			}

			w.WriteHeader(w2.status)
			w.Write(body)
		}
		return http.HandlerFunc(fn)
	}

	return f
}

func shouldAddHotReloadScript(r *http.Request) bool {
	isStream := strings.Contains(r.Header.Get("Accept"), "text/event-stream")

	if isStream {
		return false
	}

	isHTMX := r.Header.Get("HX-Request") == "true"

	if isHTMX {
		return false
	}

	return true
}

func pingLoop(r *http.Request, notify func(string)) error {
	for {
		time.Sleep(1 * time.Second)

		if r.Context().Err() != nil {
			return nil
		}

		notify("")
	}
}

const hotReloadScript = `
<script>
console.log("Hot reload enabled")

let events = new EventSource("/__ping");

let hasConnected = false;

let hasDisconnected = false;

events.onopen = () => {
	if (hasDisconnected) {
		console.log("Reloading...");
		window.location.reload();
	}

	console.log("Connected")
	hasConnected = true;
};

events.onerror = (e) => {
	hasDisconnected = true
	console.log("Disconnected")
}
</script>
`

type responseReplayer struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (c *responseReplayer) WriteHeader(status int) {
	c.status = status
}

func (c *responseReplayer) Write(b []byte) (int, error) {
	return c.buf.Write(b)
}

func (c *responseReplayer) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
