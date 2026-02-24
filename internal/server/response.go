package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// writeJSON writes v as JSON with the given HTTP status code.
// Logs a warning if JSON encoding fails.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: encoding response: %v", err)
	}
}

// writeError writes a JSON error response with the given status
// and message.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleContextError detects context.Canceled and
// context.DeadlineExceeded errors, returning true so the
// caller stops processing. It does NOT write an HTTP
// response — the withTimeout middleware handles that via
// http.TimeoutHandler (503). Writing here would race with
// the middleware's buffered response.
func handleContextError(_ http.ResponseWriter, err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}
