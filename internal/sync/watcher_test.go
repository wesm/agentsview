package sync

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// startTestWatcherNoCleanup sets up a watcher without registering
// t.Cleanup(w.Stop), for tests that explicitly exercise Stop().
func startTestWatcherNoCleanup(
	t *testing.T, onChange func([]string), debounce time.Duration,
) (*Watcher, string) {
	t.Helper()
	dir := t.TempDir()
	w, err := NewWatcher(debounce, onChange)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if _, _, err := w.WatchRecursive(dir); err != nil {
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
	w, dir := startTestWatcherNoCleanup(t, onChange, 50*time.Millisecond)
	t.Cleanup(func() { w.Stop() })
	return w, dir
}

// pollUntil dynamically polls a condition to avoid hardcoded sleeps.
func pollUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("pollUntil: condition not met within deadline")
}

func TestWatcherCallsOnChange(t *testing.T) {
	pathsCh := make(chan []string, 1)

	_, dir := startTestWatcher(t, func(paths []string) {
		select {
		case pathsCh <- paths:
		default:
		}
	})

	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	select {
	case gotPaths := <-pathsCh:
		if len(gotPaths) == 0 {
			t.Fatal("onChange called with empty paths")
		}
		if !slices.Contains(gotPaths, path) {
			t.Fatalf("onChange did not contain expected path %s, got %v", path, gotPaths)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for onChange callback")
	}
}

func TestWatcherAutoWatchesNewDirs(t *testing.T) {
	pathsCh := make(chan []string, 10)

	w, dir := startTestWatcher(t, func(paths []string) {
		pathsCh <- paths
	})

	subdir := filepath.Join(dir, "newdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	// Wait for fsnotify to process the mkdir and add the watch
	pollUntil(t, func() bool {
		return slices.Contains(w.watcher.WatchList(), subdir)
	})

	nestedPath := filepath.Join(subdir, "nested.jsonl")
	if err := os.WriteFile(nestedPath, []byte("nested"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) && !found {
		select {
		case paths := <-pathsCh:
			if slices.Contains(paths, nestedPath) {
				found = true
			}
		case <-time.After(50 * time.Millisecond):
		}
	}

	if !found {
		t.Fatal("timed out waiting for nested file change")
	}
}

func TestWatcherStopIsClean(t *testing.T) {
	w, _ := startTestWatcherNoCleanup(t, func(_ []string) {}, 50*time.Millisecond)

	stopped := make(chan struct{})
	go func() {
		w.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return in time")
	}
}

func TestWatcherStopIdempotency(t *testing.T) {
	w, _ := startTestWatcherNoCleanup(t, func(_ []string) {}, 50*time.Millisecond)

	// 1. Sequential double stop
	w.Stop()
	w.Stop()

	// 2. Concurrent stop attempts
	pathsCh2 := make(chan []string, 10)
	w2, dir2 := startTestWatcherNoCleanup(
		t, func(paths []string) {
			pathsCh2 <- paths
		}, 50*time.Millisecond,
	)

	stressPath := filepath.Join(dir2, "stress.txt")
	if err := os.WriteFile(stressPath, []byte("data"), 0o644); err != nil {
		t.Fatalf("stress write: %v", err)
	}

	// Wait for fsnotify to process it before concurrent stop
	select {
	case <-pathsCh2:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stress file to be processed")
	}

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

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Stop() timed out")
	}
}

func TestWatcherIgnoresNonWriteCreate(t *testing.T) {
	pathsCh := make(chan []string, 10)
	w, dir := startTestWatcherNoCleanup(t, func(paths []string) {
		pathsCh <- paths
	}, 10*time.Millisecond)
	t.Cleanup(func() { w.Stop() })

	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Wait for the initial write event to clear
	select {
	case <-pathsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial write event")
	}

	// Now do a chmod (should be ignored)
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	// We can manually flush and see if anything triggers, but since the
	// event won't even be recorded, flush won't do anything. We just wait a bit.
	select {
	case <-pathsCh:
		t.Fatal("onChange called for chmod event, expected it to be ignored")
	case <-time.After(100 * time.Millisecond):
		// Success
	}
}

func TestWatcherDebounceLogic(t *testing.T) {
	var mu sync.Mutex
	mockTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	pathsCh := make(chan []string, 1)

	// Use a long debounce so the internal ticker doesn't trigger naturally during the test
	w, dir := startTestWatcherNoCleanup(t, func(paths []string) {
		select {
		case pathsCh <- paths:
		default:
		}
	}, 1*time.Hour)
	t.Cleanup(func() { w.Stop() })

	w.mu.Lock()
	w.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return mockTime
	}
	w.mu.Unlock()

	path := filepath.Join(dir, "recent_dir")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	// Wait for fsnotify to process the mkdir and add the watch
	pollUntil(t, func() bool {
		return slices.Contains(w.watcher.WatchList(), path)
	})

	// 1. Flush before debounce period
	w.flush()
	select {
	case <-pathsCh:
		t.Fatal("flush should not call onChange before debounce")
	default:
	}

	// 2. Advance time past debounce period
	mu.Lock()
	mockTime = mockTime.Add(2 * time.Hour)
	mu.Unlock()

	// 3. Flush after debounce period
	w.flush()

	select {
	case gotPaths := <-pathsCh:
		if len(gotPaths) != 1 || gotPaths[0] != path {
			t.Fatalf("expected [%s], got %v", path, gotPaths)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected onChange to be called after debounce elapsed")
	}

	// 4. Flush again when empty should be a no-op
	w.flush()
	select {
	case <-pathsCh:
		t.Fatal("flush should not call onChange when empty")
	default:
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
