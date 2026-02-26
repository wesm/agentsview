package sync

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher uses fsnotify to watch session directories for changes
// and triggers a callback with debouncing.
type Watcher struct {
	onChange func(paths []string)
	watcher  *fsnotify.Watcher
	debounce time.Duration
	pending  map[string]time.Time
	mu       sync.Mutex
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	now      func() time.Time
}

// NewWatcher creates a file watcher that calls onChange when
// files are modified after the debounce period elapses.
func NewWatcher(
	debounce time.Duration, onChange func(paths []string),
) (*Watcher, error) {
	if onChange == nil {
		return nil, fmt.Errorf("onChange callback is nil: %w", os.ErrInvalid)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		onChange: onChange,
		watcher:  fsw,
		debounce: debounce,
		pending:  make(map[string]time.Time),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		now:      time.Now,
	}
	return w, nil
}

// WatchRecursive walks a directory tree and adds all
// subdirectories to the watch list. Returns the number
// of directories watched and unwatched (failed to add).
func (w *Watcher) WatchRecursive(root string) (watched int, unwatched int, err error) {
	err = filepath.WalkDir(root,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip inaccessible dirs
			}
			if d.IsDir() {
				if addErr := w.watcher.Add(path); addErr != nil {
					unwatched++
				} else {
					watched++
				}
			}
			return nil
		})
	return watched, unwatched, err
}

// Start begins processing file events in a goroutine.
func (w *Watcher) Start() {
	go w.loop()
}

// Stop stops the watcher and waits for it to finish.
func (w *Watcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.stop)
		<-w.done
		w.watcher.Close()
	})
}

func (w *Watcher) loop() {
	defer close(w.done)
	ticker := time.NewTicker(w.debounce)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)

		case <-ticker.C:
			w.flush()
		}
	}
}

// handleEvent processes a single fsnotify event, auto-watching
// newly created directories and recording pending changes.
func (w *Watcher) handleEvent(event fsnotify.Event) {
	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return
	}

	if event.Op&fsnotify.Create != 0 {
		w.watchIfDir(event.Name)
	}

	w.mu.Lock()
	w.pending[event.Name] = w.now()
	w.mu.Unlock()
}

// watchIfDir adds a path to the watch list if it is a directory.
func (w *Watcher) watchIfDir(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	_ = w.watcher.Add(path)
}

func (w *Watcher) flush() {
	w.mu.Lock()
	if len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}

	now := w.now()
	var ready []string
	for path, t := range w.pending {
		if now.Sub(t) >= w.debounce {
			ready = append(ready, path)
		}
	}

	for _, path := range ready {
		delete(w.pending, path)
	}
	w.mu.Unlock()

	if len(ready) > 0 {
		log.Printf("watcher: %d file(s) changed, triggering sync",
			len(ready))
		w.onChange(ready)
	}
}
