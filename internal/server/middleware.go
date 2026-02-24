package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// jsonError is the standard JSON error response.
type jsonError struct {
	Error string `json:"error"`
}

// withTimeout applies a write timeout to standard handlers.
// It uses http.TimeoutHandler but ensures the response is
// JSON with correct headers.
func (s *Server) withTimeout(
	h http.HandlerFunc,
) http.Handler {
	msgBytes, _ := json.Marshal(
		jsonError{Error: "request timed out"},
	)
	msg := string(msgBytes)

	inner := h
	if s.handlerDelay > 0 {
		delay := s.handlerDelay
		inner = func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(delay)
			h(w, r)
		}
	}

	handler := http.TimeoutHandler(
		inner, s.cfg.WriteTimeout, msg,
	)

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			tw := &contentTypeWrapper{
				ResponseWriter: w,
				contentType:    "application/json",
				triggerStatus:  http.StatusServiceUnavailable,
			}
			handler.ServeHTTP(tw, r)
		},
	)
}

// contentTypeWrapper intercepts WriteHeader to set Content-Type on specific status codes.
type contentTypeWrapper struct {
	http.ResponseWriter
	contentType   string
	triggerStatus int
	wroteHeader   bool
}

func (w *contentTypeWrapper) WriteHeader(code int) {
	if !w.wroteHeader {
		if code == w.triggerStatus {
			if w.ResponseWriter.Header().Get("Content-Type") == "" {
				w.ResponseWriter.Header().Set("Content-Type", w.contentType)
			}
		}
		w.ResponseWriter.WriteHeader(code)
		w.wroteHeader = true
	}
}

func (w *contentTypeWrapper) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
