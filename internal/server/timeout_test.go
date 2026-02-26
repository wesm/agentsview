package server_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestServerTimeouts starts a real HTTP server and verifies that
// streaming connections (SSE) are not closed prematurely by WriteTimeout.
func TestServerTimeouts(t *testing.T) {
	// Set a very short WriteTimeout to verify SSE is exempt.
	// If SSE were subject to this timeout, the connection would close
	// well before our 500ms wait below.
	writeTimeout := 100 * time.Millisecond
	sleepDuration := 500 * time.Millisecond // Must be > writeTimeout

	te := setup(t, withWriteTimeout(writeTimeout))

	sessionPath := te.writeProjectFile(
		t, "test-project", "watch-test.jsonl", `{"type":"user"}`,
	)

	baseURL := te.listenAndServe(t)

	// Connect to the SSE endpoint.
	url := fmt.Sprintf(
		"%s/api/v1/sessions/%s/watch", baseURL, "watch-test",
	)
	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	// Trigger an update after 500ms (> WriteTimeout of 100ms).
	// If the handler had a timeout, the body would be closed by now.
	errCh := make(chan error, 1)
	go func() {
		time.Sleep(sleepDuration)
		f, err := os.OpenFile(sessionPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			errCh <- fmt.Errorf("opening file: %w", err)
			return
		}
		defer f.Close()

		if _, err := f.WriteString("\n{\"type\":\"user\",\"content\":\"update\"}"); err != nil {
			errCh <- fmt.Errorf("writing update: %w", err)
			return
		}
	}()

	readCh := make(chan string)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "session_updated") {
				readCh <- scanner.Text()
				return
			}
		}
		close(readCh)
	}()

	select {
	case writeErr := <-errCh:
		if writeErr != nil {
			t.Fatalf("update writer failed: %v", writeErr)
		}
	case line, ok := <-readCh:
		if !ok {
			t.Fatal("stream closed before receiving update")
		}
		t.Logf("Received delayed event: %s", line)
	case <-ctx.Done():
		t.Fatal("test timed out waiting for delayed event")
	}
}
