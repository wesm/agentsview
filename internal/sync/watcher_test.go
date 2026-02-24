package sync

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// startTestWatcherNoCleanup sets up a watcher without registering
// t.Cleanup(w.Stop), for tests that explicitly exercise Stop().
func startTestWatcherNoCleanup(
	t *testing.T, onChange func([]string),
) (*Watcher, string) {
	t.Helper()
	dir := t.TempDir()
	w, err := NewWatcher(50*time.Millisecond, onChange)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if _, err := w.WatchRecursive(dir); err != nil {
		t.Fatalf("WatchRecursive: %v", err)
	}
	w.Start()
	return w, dir
}

// startTestWatcher encapsulates watcher setup and lifecycle.
func startTestWatcher(
	t *testing.T, onChange func([]string),
) (*Watcher, string) {
	t.Helper()
	w, dir := startTestWatcherNoCleanup(t, onChange)
	t.Cleanup(func() { w.Stop() })
	return w, dir
}

// Helper: waitWithTimeout standardizes waiting for a channel signal with a failure timeout
func waitWithTimeout(t *testing.T, ch <-chan struct{}, timeout time.Duration, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatal(msg)
	}
}

// pollUntil polls fn with the given interval until it returns true
// or the timeout expires.
func pollUntil(
	t *testing.T,
	timeout, interval time.Duration,
	msg string,
	fn func() bool,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(interval)
	}
	if fn() {
		return
	}
	t.Fatal(msg)
}

// newMockWatcher creates a Watcher struct for internal unit tests.
func newMockWatcher(
	debounce time.Duration, onChange func([]string),
) *Watcher {
	return &Watcher{
		debounce: debounce,
		pending:  make(map[string]time.Time),
		onChange: onChange,
	}
}

// Helper: setPending safely sets a pending file change
func setPending(w *Watcher, path string, t time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending[path] = t
}

// Helper: getPendingCount safely returns the number of pending changes
func getPendingCount(w *Watcher) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.pending)
}

// Helper: pendingContains safely checks if a path is in pending
func pendingContains(w *Watcher, path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.pending[path]
	return ok
}

func TestWatcherCallsOnChange(t *testing.T) {
	var called atomic.Bool
	var gotPaths []string
	done := make(chan struct{})

	_, dir := startTestWatcher(t, func(paths []string) {
		gotPaths = paths
		called.Store(true)
		close(done)
	})

	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	waitWithTimeout(t, done, 5*time.Second, "timed out waiting for onChange callback")

	if !called.Load() {
		t.Fatal("onChange was not called")
	}
	if len(gotPaths) == 0 {
		t.Fatal("onChange called with empty paths")
	}

	if !slices.Contains(gotPaths, path) {
		t.Fatalf("onChange did not contain expected path %s, got %v", path, gotPaths)
	}

	// t.Cleanup in startTestWatcher handles w.Stop()
}

func TestWatcherAutoWatchesNewDirs(t *testing.T) {
	var mu sync.Mutex
	var allPaths []string

	w, dir := startTestWatcher(t, func(paths []string) {
		mu.Lock()
		allPaths = append(allPaths, paths...)
		mu.Unlock()
	})

	subdir := filepath.Join(dir, "newdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	pollUntil(t, 5*time.Second, 10*time.Millisecond,
		"timed out waiting for watcher to add new directory",
		func() bool {
			return slices.Contains(w.watcher.WatchList(), subdir)
		},
	)

	nestedPath := filepath.Join(subdir, "nested.jsonl")
	if err := os.WriteFile(
		nestedPath, []byte("nested"), 0o644,
	); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pollUntil(t, 5*time.Second, 50*time.Millisecond,
		"timed out waiting for nested file change",
		func() bool {
			mu.Lock()
			defer mu.Unlock()
			return slices.Contains(allPaths, nestedPath)
		},
	)
}

func TestWatcherStopIsClean(t *testing.T) {
	w, _ := startTestWatcherNoCleanup(t, func(_ []string) {})

	stopped := make(chan struct{})
	go func() {
		w.Stop()
		close(stopped)
	}()

	waitWithTimeout(t, stopped, 5*time.Second, "Stop() did not return in time")
}

func TestWatcherStopIdempotency(t *testing.T) {
	w, _ := startTestWatcherNoCleanup(t, func(_ []string) {})

	// 1. Sequential double stop
	w.Stop()
	w.Stop()

	// 2. Concurrent stop attempts
	w2, dir2 := startTestWatcherNoCleanup(
		t, func(_ []string) {},
	)

	// Create activity so the watcher has events to process during stop
	stressPath := filepath.Join(dir2, "stress.txt")
	if err := os.WriteFile(stressPath, []byte("data"), 0o644); err != nil {
		t.Fatalf("stress write: %v", err)
	}

	// Wait until the watcher loop has consumed the fsnotify event.
	// Without this, Stop() could fire before the event is processed,
	// meaning the test never exercises "active watch during stop".
	pollUntil(t, 5*time.Second, 5*time.Millisecond,
		"timed out waiting for watcher to observe stress write",
		func() bool { return getPendingCount(w2) > 0 },
	)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			w2.Stop()
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	waitWithTimeout(t, done, 5*time.Second, "concurrent Stop() timed out")
}

func TestHandleEventIgnoresNonWriteCreate(t *testing.T) {
	w := newMockWatcher(0, nil)

	w.handleEvent(fsnotify.Event{
		Name: "file.txt", Op: fsnotify.Chmod,
	})
	w.handleEvent(fsnotify.Event{
		Name: "file.txt", Op: fsnotify.Rename,
	})
	w.handleEvent(fsnotify.Event{
		Name: "file.txt", Op: fsnotify.Remove,
	})

	if n := getPendingCount(w); n != 0 {
		t.Fatalf("expected 0 pending, got %d", n)
	}
}

func TestHandleEventRecordsPendingOnWrite(t *testing.T) {
	w := newMockWatcher(0, nil)

	w.handleEvent(fsnotify.Event{
		Name: "/tmp/test.jsonl", Op: fsnotify.Write,
	})

	if !pendingContains(w, "/tmp/test.jsonl") {
		t.Fatal("expected /tmp/test.jsonl in pending map")
	}
}

func TestFlushRespectsDebouncePeriod(t *testing.T) {
	var called atomic.Bool
	w := newMockWatcher(100*time.Millisecond,
		func(_ []string) { called.Store(true) },
	)

	setPending(w, "/tmp/recent", time.Now())

	w.flush()

	if called.Load() {
		t.Fatal("flush should not call onChange before debounce")
	}

	if n := getPendingCount(w); n != 1 {
		t.Fatalf("expected 1 pending, got %d", n)
	}
}

func TestFlushCallsOnChangeAfterDebounce(t *testing.T) {
	var gotPaths []string
	w := newMockWatcher(10*time.Millisecond,
		func(paths []string) { gotPaths = paths },
	)

	setPending(w, "/tmp/old", time.Now().Add(-50*time.Millisecond))

	w.flush()

	if len(gotPaths) != 1 || gotPaths[0] != "/tmp/old" {
		t.Fatalf("expected [/tmp/old], got %v", gotPaths)
	}

	if n := getPendingCount(w); n != 0 {
		t.Fatalf("expected 0 pending after flush, got %d", n)
	}
}

func TestFlushNoopWhenEmpty(t *testing.T) {
	var called atomic.Bool
	w := newMockWatcher(10*time.Millisecond,
		func(_ []string) { called.Store(true) },
	)

	w.flush()

	if called.Load() {
		t.Fatal("flush should not call onChange when pending is empty")
	}
}

func TestNewWatcher_NilOnChange(t *testing.T) {
	_, err := NewWatcher(time.Second, nil)
	if err == nil {
		t.Fatal("NewWatcher(nil) should return error")
	}

	if !errors.Is(err, os.ErrInvalid) {
		t.Errorf("expected wrapped os.ErrInvalid, got %v", err)
	}

	expectedMsg := "onChange callback is nil"
	if err.Error() != expectedMsg+": "+os.ErrInvalid.Error() {
		t.Errorf("expected error message to contain %q, got %q", expectedMsg, err.Error())
	}
}
