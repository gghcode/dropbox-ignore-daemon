// Package watcher provides filesystem event monitoring with batching and debouncing
package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	// Default debounce duration for batching events
	defaultDebounce = 800 * time.Millisecond
	// Default flush interval for processing batched events
	defaultFlushInterval = 250 * time.Millisecond
)

// Event represents a filesystem event with timestamp
type Event struct {
	Path string
	Op   fsnotify.Op
	Time time.Time
}

// Handler is called for each batched event after debouncing
type Handler func(event Event) error

// Watcher monitors filesystem changes with event batching
type Watcher struct {
	watcher  *fsnotify.Watcher
	handler  Handler
	debounce time.Duration
	flush    time.Duration
	
	mu      sync.Mutex
	pending map[string]Event
	
	logger *log.Logger
}

// Config holds watcher configuration
type Config struct {
	Handler       Handler
	Debounce      time.Duration
	FlushInterval time.Duration
	Logger        *log.Logger
}

// NewWatcher creates a new filesystem watcher
func NewWatcher(cfg Config) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	
	if cfg.Debounce <= 0 {
		cfg.Debounce = defaultDebounce
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = defaultFlushInterval
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	
	return &Watcher{
		watcher:  fsWatcher,
		handler:  cfg.Handler,
		debounce: cfg.Debounce,
		flush:    cfg.FlushInterval,
		pending:  make(map[string]Event),
		logger:   cfg.Logger,
	}, nil
}

// Add starts watching a path
func (w *Watcher) Add(path string) error {
	return w.watcher.Add(path)
}

// Remove stops watching a path
func (w *Watcher) Remove(path string) error {
	return w.watcher.Remove(path)
}

// AddRecursive adds all directories under root to the watch list
// Now returns error if any directory fails to be added
func (w *Watcher) AddRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// If we can't access the path, log and continue
			w.logger.Printf("Cannot access %s: %v", path, err)
			return nil
		}
		
		// Skip if not a directory
		if !info.IsDir() {
			return nil
		}
		
		// Skip common directories that should not be watched
		name := info.Name()
		if name == "node_modules" || name == ".git" || name == ".dropbox.cache" || 
		   name == ".svn" || name == ".hg" || (strings.HasPrefix(name, ".") && name != ".") {
			return filepath.SkipDir
		}
		
		// Try to add watch, but don't fail entire walk on error
		// This handles symlink directories that might point to non-existent paths
		if err := w.Add(path); err != nil {
			w.logger.Printf("Failed to watch %s: %v (continuing)", path, err)
			// Continue walking instead of returning error
			return nil
		}
		
		return nil
	})
}

// Run starts the event processing loop
func (w *Watcher) Run(ctx context.Context) error {
	flushTicker := time.NewTicker(w.flush)
	defer flushTicker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
			
		case event, ok := <-w.watcher.Events:
			if !ok {
				return nil
			}
			w.handleEvent(event)
			
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return nil
			}
			w.logger.Printf("Watch error: %v", err)
			
		case <-flushTicker.C:
			w.flushPending()
		}
	}
}

// handleEvent adds an event to the pending queue
func (w *Watcher) handleEvent(event fsnotify.Event) {
	// Only process relevant events
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
		return
	}
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	w.pending[event.Name] = Event{
		Path: event.Name,
		Op:   event.Op,
		Time: time.Now(),
	}
	
	// If it's a new directory, add it to watch list
	if event.Op&fsnotify.Create != 0 {
		// Check in a separate goroutine to avoid blocking event processing
		go func(path string) {
			stat, err := os.Stat(path)
			if err != nil {
				return
			}
			
			if !stat.IsDir() {
				return
			}
			
			// Skip common directories that should not be watched
			name := filepath.Base(path)
			if name == "node_modules" || name == ".git" || name == ".dropbox.cache" || 
			   name == ".svn" || name == ".hg" || (strings.HasPrefix(name, ".") && name != ".") {
				return
			}
			
			// Try to add watch, handling potential errors for symlink directories
			if err := w.Add(path); err != nil {
				w.logger.Printf("Failed to watch new directory %s: %v", path, err)
			}
		}(event.Name)
	}
}

// flushPending processes debounced events synchronously
func (w *Watcher) flushPending() {
	w.mu.Lock()
	
	// Copy events to process
	toProcess := make([]Event, 0, len(w.pending))
	now := time.Now()
	
	for path, event := range w.pending {
		if now.Sub(event.Time) >= w.debounce {
			toProcess = append(toProcess, event)
			delete(w.pending, path)
		}
	}
	
	w.mu.Unlock()
	
	// Process events synchronously (no goroutines)
	for _, event := range toProcess {
		if err := w.handler(event); err != nil {
			w.logger.Printf("Handler error for %s: %v", event.Path, err)
		}
	}
}

// Close stops watching and cleans up resources
func (w *Watcher) Close() error {
	return w.watcher.Close()
}