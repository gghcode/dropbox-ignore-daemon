package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWatcherEventBatching(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Track handled events
	var mu sync.Mutex
	handled := make(map[string]int)
	
	handler := func(event Event) error {
		mu.Lock()
		handled[event.Path]++
		mu.Unlock()
		return nil
	}
	
	// Create watcher with short intervals for testing
	w, err := NewWatcher(Config{
		Handler:       handler,
		Debounce:      100 * time.Millisecond,
		FlushInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()
	
	// Add directory to watch
	if err := w.Add(tmpDir); err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}
	
	// Start watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go w.Run(ctx)
	
	// Create multiple events quickly (should be batched)
	testFile := filepath.Join(tmpDir, "test.txt")
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	
	// Wait for debounce
	time.Sleep(200 * time.Millisecond)
	
	// Check that events were batched (should be 1, not 5)
	mu.Lock()
	count := handled[testFile]
	mu.Unlock()
	
	if count != 1 {
		t.Errorf("Expected 1 batched event, got %d", count)
	}
}

func TestWatcherDebounce(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Track event times
	var mu sync.Mutex
	var eventTime time.Time
	
	handler := func(event Event) error {
		mu.Lock()
		eventTime = time.Now()
		mu.Unlock()
		return nil
	}
	
	// Create watcher with 200ms debounce
	w, err := NewWatcher(Config{
		Handler:       handler,
		Debounce:      200 * time.Millisecond,
		FlushInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()
	
	// Add directory to watch
	if err := w.Add(tmpDir); err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}
	
	// Start watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go w.Run(ctx)
	
	// Create event
	testFile := filepath.Join(tmpDir, "test.txt")
	startTime := time.Now()
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	
	// Wait for event to be processed
	time.Sleep(300 * time.Millisecond)
	
	// Check debounce timing
	mu.Lock()
	elapsed := eventTime.Sub(startTime)
	mu.Unlock()
	
	if elapsed < 200*time.Millisecond {
		t.Errorf("Event processed too early: %v", elapsed)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("Event processed too late: %v", elapsed)
	}
}

func TestWatcherRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create nested directory structure
	subDir1 := filepath.Join(tmpDir, "sub1")
	subDir2 := filepath.Join(tmpDir, "sub1", "sub2")
	os.MkdirAll(subDir2, 0755)
	
	// Track events
	var mu sync.Mutex
	events := make(map[string]bool)
	
	handler := func(event Event) error {
		mu.Lock()
		events[event.Path] = true
		mu.Unlock()
		return nil
	}
	
	w, err := NewWatcher(Config{
		Handler:       handler,
		Debounce:      50 * time.Millisecond,
		FlushInterval: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()
	
	// Add recursive watch
	if err := w.AddRecursive(tmpDir); err != nil {
		t.Fatalf("Failed to add recursive watch: %v", err)
	}
	
	// Start watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go w.Run(ctx)
	
	// Create files in different directories
	files := []string{
		filepath.Join(tmpDir, "root.txt"),
		filepath.Join(subDir1, "sub1.txt"),
		filepath.Join(subDir2, "sub2.txt"),
	}
	
	for _, file := range files {
		if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to write file %s: %v", file, err)
		}
	}
	
	// Wait for events
	time.Sleep(150 * time.Millisecond)
	
	// Check all files triggered events
	mu.Lock()
	defer mu.Unlock()
	
	for _, file := range files {
		if !events[file] {
			t.Errorf("No event received for %s", file)
		}
	}
}

func TestWatcherNewDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Track events
	var mu sync.Mutex
	events := make(map[string]bool)
	
	handler := func(event Event) error {
		mu.Lock()
		events[event.Path] = true
		mu.Unlock()
		return nil
	}
	
	w, err := NewWatcher(Config{
		Handler:       handler,
		Debounce:      50 * time.Millisecond,
		FlushInterval: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()
	
	// Watch root directory
	if err := w.Add(tmpDir); err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}
	
	// Start watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go w.Run(ctx)
	
	// Create new directory
	newDir := filepath.Join(tmpDir, "newdir")
	if err := os.Mkdir(newDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	
	// Wait for directory to be added to watch
	time.Sleep(100 * time.Millisecond)
	
	// Create file in new directory
	newFile := filepath.Join(newDir, "test.txt")
	if err := os.WriteFile(newFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	
	// Wait for events
	time.Sleep(100 * time.Millisecond)
	
	// Check events received
	mu.Lock()
	defer mu.Unlock()
	
	if !events[newDir] {
		t.Error("No event for new directory")
	}
	if !events[newFile] {
		t.Error("No event for file in new directory")
	}
}

// TestWatcherSymbolicLinks tests watching symbolic link directories
func TestWatcherSymbolicLinks(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Track events
	var mu sync.Mutex
	events := make(map[string]bool)
	
	handler := func(event Event) error {
		mu.Lock()
		events[event.Path] = true
		mu.Unlock()
		return nil
	}
	
	w, err := NewWatcher(Config{
		Handler:       handler,
		Debounce:      50 * time.Millisecond,
		FlushInterval: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()
	
	// Create real directory
	realDir := filepath.Join(tmpDir, "realdir")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("Failed to create real directory: %v", err)
	}
	
	// Create symbolic link to directory
	linkDir := filepath.Join(tmpDir, "linkdir")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("Failed to create directory symlink: %v", err)
	}
	
	// Add watches for both real and symlink directories
	if err := w.Add(realDir); err != nil {
		t.Fatalf("Failed to watch real directory: %v", err)
	}
	
	// Watching symlink directory might fail, but should not crash
	linkWatchErr := w.Add(linkDir)
	if linkWatchErr != nil {
		t.Logf("Expected behavior: Failed to watch symlink directory: %v", linkWatchErr)
	}
	
	// Start watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go func() {
		if err := w.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("Watcher error: %v", err)
		}
	}()
	
	// Wait for watcher to start
	time.Sleep(50 * time.Millisecond)
	
	// Create file in real directory
	realFile := filepath.Join(realDir, "test.txt")
	if err := os.WriteFile(realFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	
	// Wait for events
	time.Sleep(100 * time.Millisecond)
	
	// Check events
	mu.Lock()
	defer mu.Unlock()
	
	if !events[realFile] {
		t.Error("No event for file created in real directory")
	}
	
	// If symlink watching is supported, we might also get an event through the symlink
	linkFile := filepath.Join(linkDir, "test.txt")
	if events[linkFile] {
		t.Log("Symlink directory watching is supported - got event through symlink")
	}
}

// TestWatcherAddRecursiveWithSymlinks tests AddRecursive with symbolic links
func TestWatcherAddRecursiveWithSymlinks(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create directory structure with symlinks
	realDir := filepath.Join(tmpDir, "real")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("Failed to create real directory: %v", err)
	}
	
	subDir := filepath.Join(realDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}
	
	// Create symbolic link to directory
	linkDir := filepath.Join(tmpDir, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("Failed to create directory symlink: %v", err)
	}
	
	// Create broken symlink
	brokenLink := filepath.Join(tmpDir, "broken")
	if err := os.Symlink("/nonexistent/path", brokenLink); err != nil {
		t.Fatalf("Failed to create broken symlink: %v", err)
	}
	
	handler := func(event Event) error { return nil }
	
	w, err := NewWatcher(Config{
		Handler: handler,
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()
	
	// AddRecursive should handle symlinks gracefully
	if err := w.AddRecursive(tmpDir); err != nil {
		t.Fatalf("AddRecursive failed: %v", err)
	}
	
	// The function should complete without crashing, even with symlinks and broken links
	t.Log("AddRecursive completed successfully with symlinks")
}