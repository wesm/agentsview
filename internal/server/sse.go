package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const sseWriteTimeout = 3 * time.Second

// SSEStream manages a Server-Sent Events connection.
type SSEStream struct {
	w http.ResponseWriter
	f http.Flusher
}

// NewSSEStream initializes an SSE connection by setting the
// required headers and flushing them to the client. Returns an
// error if the ResponseWriter does not support streaming.
func NewSSEStream(w http.ResponseWriter) (*SSEStream, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	f.Flush()
	return &SSEStream{w: w, f: f}, nil
}

// Send writes an SSE event with the given name and string data.
// It returns false when the write fails.
func (s *SSEStream) Send(event, data string) bool {
	// Apply a bounded write deadline when supported so a stalled
	// client cannot block handlers forever.
	rc := http.NewResponseController(s.w)
	_ = rc.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
	defer func() { _ = rc.SetWriteDeadline(time.Time{}) }()

	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		log.Printf("SSE write error for %q: %v", event, err)
		return false
	}
	s.f.Flush()
	return true
}

// SendJSON writes an SSE event with JSON-serialized data.
// Logs and skips the event if marshaling fails.
func (s *SSEStream) SendJSON(event string, v any) bool {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("SSE marshal error for %q: %v", event, err)
		return false
	}
	return s.Send(event, string(data))
}

// ForceWriteDeadlineNow asks the underlying writer (when
// supported) to expire write deadlines immediately. This is used
// during shutdown to unblock stalled writes.
func (s *SSEStream) ForceWriteDeadlineNow() {
	rc := http.NewResponseController(s.w)
	_ = rc.SetWriteDeadline(time.Now())
}
