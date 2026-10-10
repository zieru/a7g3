package grapari

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Watcher monitors the knowledge source file for changes and triggers incremental sync with debouncing.
type Watcher struct {
	mu           sync.Mutex
	sourcePath   string
	syncer       *Syncer
	pollInterval time.Duration
	debounceDur  time.Duration
	lastModTime  time.Time
	lastFileSize int64
	verbose      bool
}

// NewWatcher creates a cross-platform file watcher for GraPARI knowledge files.
func NewWatcher(sourcePath string, syncer *Syncer, verbose bool) *Watcher {
	return &Watcher{
		sourcePath:   filepath.Clean(sourcePath),
		syncer:       syncer,
		pollInterval: 2 * time.Second,
		debounceDur:  1500 * time.Millisecond,
		verbose:      verbose,
	}
}

// Start begins watching the target file in a background goroutine.
func (w *Watcher) Start(ctx context.Context) {
	if stat, err := os.Stat(w.sourcePath); err == nil {
		w.lastModTime = stat.ModTime()
		w.lastFileSize = stat.Size()
	}

	go w.watchLoop(ctx)
}

func (w *Watcher) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	if w.verbose {
		log.Printf("[grapari-watcher] watching %s (poll interval: %v, debounce: %v)\n", w.sourcePath, w.pollInterval, w.debounceDur)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.checkChange(ctx)
		}
	}
}

func (w *Watcher) checkChange(ctx context.Context) {
	stat, err := os.Stat(w.sourcePath)
	if err != nil {
		return // File might temporarily be locked or inaccessible during Excel save
	}

	modTime := stat.ModTime()
	size := stat.Size()

	if modTime.Equal(w.lastModTime) && size == w.lastFileSize {
		return
	}

	// File changed. Wait for debounce duration so writing is completed.
	time.Sleep(w.debounceDur)

	// Re-stat after debounce
	statAfter, err := os.Stat(w.sourcePath)
	if err != nil {
		return
	}

	w.mu.Lock()
	w.lastModTime = statAfter.ModTime()
	w.lastFileSize = statAfter.Size()
	w.mu.Unlock()

	if w.verbose {
		log.Printf("[grapari-watcher] detected change in %s, triggering incremental sync...\n", w.sourcePath)
	}

	ins, upd, del, err := w.syncer.Sync(ctx, false)
	if err != nil {
		log.Printf("[grapari-watcher] sync error: %v\n", err)
		return
	}

	if w.verbose || ins > 0 || upd > 0 || del > 0 {
		log.Printf("[grapari-watcher] sync completed: +%d inserted, ~%d updated, -%d deleted\n", ins, upd, del)
	}
}
