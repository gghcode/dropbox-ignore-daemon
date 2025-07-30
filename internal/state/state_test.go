package state

import (
	"os"
	"testing"
	"time"
)

func getFileInfo(t *testing.T, path string) os.FileInfo {
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	return info
}

func TestCacheBasicOperations(t *testing.T) {
	cache := NewCache(1 * time.Minute)
	
	// Create test file
	tmpFile := t.TempDir() + "/test.txt"
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	info := getFileInfo(t, tmpFile)
	
	// Test Has on empty cache
	if cache.Has(tmpFile, info) {
		t.Error("Empty cache should not have entry")
	}
	
	// Test Add
	cache.Add(tmpFile, info)
	if !cache.Has(tmpFile, info) {
		t.Error("Cache should have entry after Add")
	}
	
	// Test Size
	if cache.Size() != 1 {
		t.Errorf("Cache size should be 1, got %d", cache.Size())
	}
	
	// Test Remove
	cache.Remove(tmpFile)
	if cache.Has(tmpFile, info) {
		t.Error("Cache should not have entry after Remove")
	}
}

func TestCacheModificationDetection(t *testing.T) {
	cache := NewCache(1 * time.Minute)
	
	// Create test file
	tmpFile := t.TempDir() + "/test.txt"
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	info1 := getFileInfo(t, tmpFile)
	cache.Add(tmpFile, info1)
	
	// Modify file
	time.Sleep(10 * time.Millisecond) // Ensure mtime changes
	if err := os.WriteFile(tmpFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}
	
	info2 := getFileInfo(t, tmpFile)
	
	// Cache should detect modification
	if cache.Has(tmpFile, info2) {
		t.Error("Cache should detect file modification")
	}
}

func TestCacheTTL(t *testing.T) {
	cache := NewCache(50 * time.Millisecond)
	
	// Create test file
	tmpFile := t.TempDir() + "/test.txt"
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	info := getFileInfo(t, tmpFile)
	cache.Add(tmpFile, info)
	
	// Entry should exist
	if !cache.Has(tmpFile, info) {
		t.Error("Cache should have entry")
	}
	
	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)
	
	// Entry should be expired
	if cache.Has(tmpFile, info) {
		t.Error("Cache entry should be expired")
	}
}

func TestCacheClean(t *testing.T) {
	cache := NewCache(50 * time.Millisecond)
	
	// Create test files
	tmpDir := t.TempDir()
	files := []string{
		tmpDir + "/test1.txt",
		tmpDir + "/test2.txt",
		tmpDir + "/test3.txt",
	}
	
	for _, file := range files {
		if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		info := getFileInfo(t, file)
		cache.Add(file, info)
	}
	
	// All entries should exist
	if cache.Size() != 3 {
		t.Errorf("Cache should have 3 entries, got %d", cache.Size())
	}
	
	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)
	
	// Clean expired entries
	cache.Clean()
	
	// All entries should be removed
	if cache.Size() != 0 {
		t.Errorf("Cache should be empty after clean, got %d entries", cache.Size())
	}
}

func TestCacheLazyEviction(t *testing.T) {
	cache := NewCache(50 * time.Millisecond)
	
	// Create test file
	tmpFile := t.TempDir() + "/test.txt"
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	info := getFileInfo(t, tmpFile)
	cache.Add(tmpFile, info)
	
	// Entry should exist
	if cache.Size() != 1 {
		t.Error("Cache should have 1 entry")
	}
	
	// Entry should be found initially
	if !cache.Has(tmpFile, info) {
		t.Error("Cache should have the entry")
	}
	
	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)
	
	// Has() should return false and evict the entry
	if cache.Has(tmpFile, info) {
		t.Error("Cache should not have expired entry")
	}
	
	// Size should be 0 after lazy eviction
	if cache.Size() != 0 {
		t.Errorf("Cache should be empty after lazy eviction, got %d entries", cache.Size())
	}
}