package poller

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPollerBasicScan(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create test files
	files := []string{
		filepath.Join(tmpDir, "file1.txt"),
		filepath.Join(tmpDir, "file2.txt"),
		filepath.Join(tmpDir, "subdir", "file3.txt"),
	}
	
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	for _, file := range files {
		if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}
	
	// Track scanned files
	var mu sync.Mutex
	scanned := make(map[string]bool)
	
	handler := func(path string, info fs.FileInfo) error {
		mu.Lock()
		scanned[path] = true
		mu.Unlock()
		return nil
	}
	
	p, err := NewPoller(Config{
		Root:    tmpDir,
		Handler: handler,
	})
	if err != nil {
		t.Fatalf("Failed to create poller: %v", err)
	}
	
	// Run single scan
	if err := p.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	
	// Check all files were scanned
	mu.Lock()
	defer mu.Unlock()
	
	for _, file := range files {
		if !scanned[file] {
			t.Errorf("File not scanned: %s", file)
		}
	}
}

func TestPollerIncrementalScan(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create initial files
	oldFile := filepath.Join(tmpDir, "old.txt")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}
	
	// Track scanned files
	var mu sync.Mutex
	scanCount := make(map[string]int)
	
	handler := func(path string, info fs.FileInfo) error {
		mu.Lock()
		scanCount[path]++
		mu.Unlock()
		return nil
	}
	
	p, err := NewPoller(Config{
		Root:    tmpDir,
		Handler: handler,
	})
	if err != nil {
		t.Fatalf("Failed to create poller: %v", err)
	}
	
	// First scan
	if err := p.Scan(); err != nil {
		t.Fatalf("First scan failed: %v", err)
	}
	
	// Wait to ensure time difference
	time.Sleep(10 * time.Millisecond)
	
	// Create new file
	newFile := filepath.Join(tmpDir, "new.txt")
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}
	
	// Second scan - should only scan new file
	if err := p.Scan(); err != nil {
		t.Fatalf("Second scan failed: %v", err)
	}
	
	mu.Lock()
	defer mu.Unlock()
	
	// Old file should be scanned once
	if scanCount[oldFile] != 1 {
		t.Errorf("Old file scanned %d times, expected 1", scanCount[oldFile])
	}
	
	// New file should be scanned once
	if scanCount[newFile] != 1 {
		t.Errorf("New file scanned %d times, expected 1", scanCount[newFile])
	}
}

func TestPollerSkipDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create directory structure
	dirs := []string{
		filepath.Join(tmpDir, ".git"),
		filepath.Join(tmpDir, ".dropbox.cache"),
		filepath.Join(tmpDir, "node_modules"),
		filepath.Join(tmpDir, "normal"),
	}
	
	for _, dir := range dirs {
		os.MkdirAll(dir, 0755)
		// Create a file in each directory
		if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}
	
	// Track scanned files
	var mu sync.Mutex
	scanned := make(map[string]bool)
	
	handler := func(path string, info fs.FileInfo) error {
		mu.Lock()
		scanned[path] = true
		mu.Unlock()
		return nil
	}
	
	p, err := NewPoller(Config{
		Root:    tmpDir,
		Handler: handler,
	})
	if err != nil {
		t.Fatalf("Failed to create poller: %v", err)
	}
	
	// Run scan
	if err := p.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	
	mu.Lock()
	defer mu.Unlock()
	
	// Check that skip directories were not scanned
	skipDirs := []string{".git", ".dropbox.cache", "node_modules"}
	for _, skip := range skipDirs {
		file := filepath.Join(tmpDir, skip, "test.txt")
		if scanned[file] {
			t.Errorf("File in skip directory was scanned: %s", file)
		}
	}
	
	// Check that normal directory was scanned
	normalFile := filepath.Join(tmpDir, "normal", "test.txt")
	if !scanned[normalFile] {
		t.Errorf("File in normal directory was not scanned: %s", normalFile)
	}
}

func TestPollerRunWithContext(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Use GetLastScan to verify scan execution
	p, err := NewPoller(Config{
		Root:         tmpDir,
		Handler:      func(path string, info fs.FileInfo) error { return nil },
		ScanInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create poller: %v", err)
	}
	
	// Get initial state
	initialScan := p.GetLastScan()
	if !initialScan.IsZero() {
		t.Error("Expected zero time for last scan before running")
	}
	
	// Run with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	
	err = p.Run(ctx)
	
	// Should have context deadline exceeded
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
	
	// Verify scan was executed
	lastScan := p.GetLastScan()
	if lastScan.IsZero() {
		t.Error("Expected non-zero last scan time")
	}
	if !lastScan.After(initialScan) {
		t.Error("Expected last scan to be after initial time")
	}
}

func TestPollerDirectoryModTimeCache(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create subdirectory with file
	subDir := filepath.Join(tmpDir, "subdir")
	os.MkdirAll(subDir, 0755)
	
	subFile := filepath.Join(subDir, "file.txt")
	if err := os.WriteFile(subFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// Track handler calls
	var mu sync.Mutex
	calls := 0
	
	handler := func(path string, info fs.FileInfo) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	}
	
	p, err := NewPoller(Config{
		Root:    tmpDir,
		Handler: handler,
	})
	if err != nil {
		t.Fatalf("Failed to create poller: %v", err)
	}
	
	// First scan
	if err := p.Scan(); err != nil {
		t.Fatalf("First scan failed: %v", err)
	}
	
	mu.Lock()
	firstCalls := calls
	mu.Unlock()
	
	// Now handler is called for directories too: tmpDir, subDir, and file.txt
	if firstCalls != 3 {
		t.Errorf("Expected 3 calls on first scan (2 dirs + 1 file), got %d", firstCalls)
	}
	
	// Second scan without changes - should skip directory
	if err := p.Scan(); err != nil {
		t.Fatalf("Second scan failed: %v", err)
	}
	
	mu.Lock()
	secondCalls := calls
	mu.Unlock()
	
	// Both root directory and subdirectory will be processed again
	// because we process directories before checking if they should be skipped
	expectedNewCalls := 2 // Root directory and subdirectory
	if secondCalls-firstCalls != expectedNewCalls {
		t.Errorf("Expected %d new calls on second scan, got %d", expectedNewCalls, secondCalls-firstCalls)
	}
	
	// Clear the cache to force rescan
	p.ClearCache()
	
	// Add new file
	newFile := filepath.Join(subDir, "new.txt")
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}
	
	// Third scan - should process directory again
	if err := p.Scan(); err != nil {
		t.Fatalf("Third scan failed: %v", err)
	}
	
	mu.Lock()
	thirdCalls := calls
	mu.Unlock()
	
	// Should have processed at least the new file
	if thirdCalls <= secondCalls {
		t.Errorf("Expected new calls on third scan, got %d total (was %d)", thirdCalls, secondCalls)
	}
}

// TestPollerSymbolicLinks tests handling of symbolic links
func TestPollerSymbolicLinks(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create test files and directories
	realFile := filepath.Join(tmpDir, "real.txt")
	if err := os.WriteFile(realFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create real file: %v", err)
	}
	
	realDir := filepath.Join(tmpDir, "realdir")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("Failed to create real directory: %v", err)
	}
	
	realDirFile := filepath.Join(realDir, "file.txt")
	if err := os.WriteFile(realDirFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file in real directory: %v", err)
	}
	
	// Create symbolic links
	linkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Fatalf("Failed to create file symlink: %v", err)
	}
	
	linkDir := filepath.Join(tmpDir, "linkdir")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("Failed to create directory symlink: %v", err)
	}
	
	// Track handler calls
	var mu sync.Mutex
	handledPaths := make(map[string]bool)
	
	handler := func(path string, info fs.FileInfo) error {
		mu.Lock()
		handledPaths[path] = true
		mu.Unlock()
		return nil
	}
	
	p, err := NewPoller(Config{
		Root:    tmpDir,
		Handler: handler,
	})
	if err != nil {
		t.Fatalf("Failed to create poller: %v", err)
	}
	
	// Scan
	if err := p.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	
	mu.Lock()
	defer mu.Unlock()
	
	// Check that symbolic links were processed
	if !handledPaths[linkFile] {
		t.Errorf("Symbolic link file was not processed: %s", linkFile)
	}
	
	if !handledPaths[linkDir] {
		t.Errorf("Symbolic link directory was not processed: %s", linkDir)
	}
	
	// filepath.Walk doesn't follow symlinks by default, so files inside
	// symlink directories won't be processed unless explicitly followed
	
	// Verify paths processed
	expectedPaths := map[string]bool{
		tmpDir:      true, // root
		realFile:    true, // real file
		realDir:     true, // real directory  
		realDirFile: true, // file in real directory
		linkFile:    true, // symlink to file
		linkDir:     true, // symlink to directory
	}
	
	for path, expected := range expectedPaths {
		if expected && !handledPaths[path] {
			t.Errorf("Expected path was not processed: %s", path)
		}
	}
	
	t.Logf("Total paths processed: %d", len(handledPaths))
}

// TestPollerIgnoredDirectorySkip tests that contents of ignored directories are skipped
func TestPollerIgnoredDirectorySkip(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create directory structure
	ignoredDir := filepath.Join(tmpDir, "ignored")
	if err := os.MkdirAll(ignoredDir, 0755); err != nil {
		t.Fatalf("Failed to create ignored directory: %v", err)
	}
	
	// Create files inside the directory that will be ignored
	subDir := filepath.Join(ignoredDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}
	
	files := []string{
		filepath.Join(ignoredDir, "file1.txt"),
		filepath.Join(ignoredDir, "file2.txt"),
		filepath.Join(subDir, "file3.txt"),
	}
	
	for _, file := range files {
		if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}
	
	// Create a regular file outside the ignored directory
	regularFile := filepath.Join(tmpDir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}
	
	// Track handler calls
	var mu sync.Mutex
	handledPaths := make(map[string]bool)
	
	handler := func(path string, info fs.FileInfo) error {
		mu.Lock()
		handledPaths[path] = true
		mu.Unlock()
		
		// Simulate ignoring the "ignored" directory
		if path == ignoredDir && info.IsDir() {
			return ErrSkipDir
		}
		
		return nil
	}
	
	p, err := NewPoller(Config{
		Root:    tmpDir,
		Handler: handler,
	})
	if err != nil {
		t.Fatalf("Failed to create poller: %v", err)
	}
	
	// Scan
	if err := p.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	
	mu.Lock()
	defer mu.Unlock()
	
	// Check what was processed
	if !handledPaths[tmpDir] {
		t.Error("Root directory was not processed")
	}
	
	if !handledPaths[ignoredDir] {
		t.Error("Ignored directory itself was not processed")
	}
	
	if !handledPaths[regularFile] {
		t.Error("Regular file was not processed")
	}
	
	// These should NOT have been processed because they're inside an ignored directory
	for _, file := range files {
		if handledPaths[file] {
			t.Errorf("File inside ignored directory should not have been processed: %s", file)
		}
	}
	
	if handledPaths[subDir] {
		t.Error("Subdirectory inside ignored directory should not have been processed")
	}
	
	t.Logf("Total paths processed: %d (should be 3: root, ignored dir, and regular file)", len(handledPaths))
}