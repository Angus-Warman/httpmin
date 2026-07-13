package response

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// ErrStreamClosed is returned when a write is attempted on a closed stream.
var ErrStreamClosed = errors.New("stream closed")

type StreamResponse struct {
	w       http.ResponseWriter
	r       *http.Request
	flusher http.Flusher

	mu sync.Mutex // guards writes to w, since http.ResponseWriter isn't safe for concurrent use
}

func (s *StreamResponse) Send(message string) error {
	if s.Closed() {
		return errors.New("stream closed")
	}

	formatted := formatSSE("", message)

	return s.safeWrite(formatted)
}

func (s *StreamResponse) SendEvent(eventName, message string) error {
	if s.Closed() {
		return ErrStreamClosed
	}

	formatted := formatSSE(eventName, message)

	return s.safeWrite(formatted)
}

func (s *StreamResponse) safeWrite(message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check again under the lock
	if s.Closed() {
		return ErrStreamClosed
	}

	_, err := fmt.Fprint(s.w, message)

	if err != nil {
		return err
	}

	s.flusher.Flush()

	return nil
}

func (s *StreamResponse) Closed() bool {
	select {
	case <-s.r.Context().Done():
		return true
	default:
		return false
	}
}

func formatSSE(eventName, message string) string {
	var sb strings.Builder

	if eventName != "" {
		fmt.Fprintf(&sb, "event: %v\n", eventName)
	}

	for line := range strings.SplitSeq(message, "\n") {
		fmt.Fprintf(&sb, "data: %v\n", line)
	}

	sb.WriteString("\n")
	return sb.String()
}

// Turns a long-running operation into a server-sent event stream
func Stream(w http.ResponseWriter, r *http.Request) *StreamResponse {
	flusher, ok := w.(http.Flusher)

	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return nil
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// nginx
	w.Header().Set("X-Accel-Buffering", "no")

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &StreamResponse{
		w:       w,
		r:       r,
		flusher: flusher,
	}
}
