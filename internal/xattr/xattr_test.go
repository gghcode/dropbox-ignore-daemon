package xattr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetGetRemoveIgnored(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	
	// Create test file
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// Test IsIgnored on new file
	ignored, err := IsIgnored(testFile)
	if err != nil {
		t.Fatalf("IsIgnored failed: %v", err)
	}
	if ignored {
		t.Error("New file should not be ignored")
	}
	
	// Test SetIgnored
	if err := SetIgnored(testFile); err != nil {
		t.Fatalf("SetIgnored failed: %v", err)
	}
	
	// Verify file is now ignored
	ignored, err = IsIgnored(testFile)
	if err != nil {
		t.Fatalf("IsIgnored failed after set: %v", err)
	}
	if !ignored {
		t.Error("File should be ignored after SetIgnored")
	}
	
	// Test RemoveIgnored
	if err := RemoveIgnored(testFile); err != nil {
		t.Fatalf("RemoveIgnored failed: %v", err)
	}
	
	// Verify attribute is removed
	ignored, err = IsIgnored(testFile)
	if err != nil {
		t.Fatalf("IsIgnored failed after remove: %v", err)
	}
	if ignored {
		t.Error("File should not be ignored after RemoveIgnored")
	}
}

func TestIgnoredDirectory(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "testdir")
	
	// Create test directory
	if err := os.Mkdir(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	
	// Test setting ignored on directory
	if err := SetIgnored(testDir); err != nil {
		t.Fatalf("SetIgnored on directory failed: %v", err)
	}
	
	// Verify directory is ignored
	ignored, err := IsIgnored(testDir)
	if err != nil {
		t.Fatalf("IsIgnored on directory failed: %v", err)
	}
	if !ignored {
		t.Error("Directory should be ignored")
	}
}

func TestRemoveNonExistentAttribute(t *testing.T) {
	// Create a temporary file
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// RemoveIgnored should not fail on file without attribute
	if err := RemoveIgnored(tmpFile); err != nil {
		t.Errorf("RemoveIgnored should not fail on file without attribute: %v", err)
	}
}

func TestSymbolicLinks(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a real file
	realFile := filepath.Join(tmpDir, "real.txt")
	if err := os.WriteFile(realFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create real file: %v", err)
	}
	
	// Create a symbolic link to the file
	linkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Fatalf("Failed to create file symlink: %v", err)
	}
	
	// Test setting ignored on symlink
	if err := SetIgnored(linkFile); err != nil {
		t.Fatalf("SetIgnored on symlink failed: %v", err)
	}
	
	// The attribute should be set on the target file, not the symlink itself
	// Check if the real file has the attribute
	ignored, err := IsIgnored(realFile)
	if err != nil {
		t.Fatalf("IsIgnored on real file failed: %v", err)
	}
	if !ignored {
		t.Error("Real file should be ignored after setting attribute through symlink")
	}
	
	// Check through symlink - should also show as ignored
	ignoredLink, err := IsIgnored(linkFile)
	if err != nil {
		t.Fatalf("IsIgnored on symlink failed: %v", err)
	}
	if !ignoredLink {
		t.Error("Symlink should report as ignored")
	}
	
	// Test with directory symlinks
	realDir := filepath.Join(tmpDir, "realdir")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatalf("Failed to create real directory: %v", err)
	}
	
	linkDir := filepath.Join(tmpDir, "linkdir")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("Failed to create directory symlink: %v", err)
	}
	
	// Set ignored on directory symlink
	if err := SetIgnored(linkDir); err != nil {
		t.Fatalf("SetIgnored on directory symlink failed: %v", err)
	}
	
	// Check if real directory has the attribute
	ignoredDir, err := IsIgnored(realDir)
	if err != nil {
		t.Fatalf("IsIgnored on real directory failed: %v", err)
	}
	if !ignoredDir {
		t.Error("Real directory should be ignored after setting attribute through symlink")
	}
}

func TestBrokenSymbolicLinks(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a broken symbolic link
	brokenLink := filepath.Join(tmpDir, "broken")
	if err := os.Symlink("/nonexistent/path", brokenLink); err != nil {
		t.Fatalf("Failed to create broken symlink: %v", err)
	}
	
	// Setting attribute on broken symlink should fail gracefully
	err := SetIgnored(brokenLink)
	if err == nil {
		t.Error("Expected error when setting attribute on broken symlink")
	}
	
	// Checking attribute on broken symlink should also handle error gracefully
	ignored, err := IsIgnored(brokenLink)
	if err == nil {
		t.Error("Expected error when checking attribute on broken symlink")
	}
	if ignored {
		t.Error("Broken symlink should not report as ignored")
	}
}