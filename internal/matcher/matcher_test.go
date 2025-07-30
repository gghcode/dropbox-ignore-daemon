package matcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatcherBasic(t *testing.T) {
	// Create test directory structure
	tmpDir := t.TempDir()
	
	// Create .dropboxignore file
	ignoreContent := `# Test ignore patterns
node_modules/
*.log
.cache/
!important.log
`
	ignoreFile := filepath.Join(tmpDir, ".dropboxignore")
	if err := os.WriteFile(ignoreFile, []byte(ignoreContent), 0644); err != nil {
		t.Fatalf("Failed to create ignore file: %v", err)
	}
	
	// Create matcher
	m, err := NewMatcher(10)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	
	// Test cases
	tests := []struct {
		path   string
		ignore bool
	}{
		{filepath.Join(tmpDir, "node_modules", "package.json"), true},
		{filepath.Join(tmpDir, "src", "main.go"), false},
		{filepath.Join(tmpDir, "debug.log"), true},
		{filepath.Join(tmpDir, "important.log"), false}, // Negated pattern
		{filepath.Join(tmpDir, ".cache", "data"), true},
	}
	
	// Create test files/directories
	os.MkdirAll(filepath.Join(tmpDir, "node_modules"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, ".cache"), 0755)
	
	for _, tt := range tests {
		shouldIgnore, err := m.ShouldIgnore(tt.path)
		if err != nil {
			t.Errorf("Error checking %s: %v", tt.path, err)
			continue
		}
		if shouldIgnore != tt.ignore {
			t.Errorf("Path %s: expected ignore=%v, got %v", tt.path, tt.ignore, shouldIgnore)
		}
	}
}

func TestMatcherNested(t *testing.T) {
	// Create nested directory structure
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "project")
	os.MkdirAll(subDir, 0755)
	
	// Create root ignore file
	rootIgnoreContent := `*.tmp
build/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".dropboxignore"), []byte(rootIgnoreContent), 0644); err != nil {
		t.Fatalf("Failed to create root ignore file: %v", err)
	}
	
	// Create project ignore file
	projectIgnoreContent := `dist/
*.cache
`
	if err := os.WriteFile(filepath.Join(subDir, ".dropboxignore"), []byte(projectIgnoreContent), 0644); err != nil {
		t.Fatalf("Failed to create project ignore file: %v", err)
	}
	
	m, err := NewMatcher(10)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	
	// Test that closest .dropboxignore file is used
	tests := []struct {
		path   string
		ignore bool
	}{
		{filepath.Join(tmpDir, "test.tmp"), true},      // Root pattern
		{filepath.Join(tmpDir, "build", "app"), true},  // Root pattern
		{filepath.Join(subDir, "test.tmp"), false},     // Not in project ignore
		{filepath.Join(subDir, "dist", "app.js"), true}, // Project pattern
		{filepath.Join(subDir, "data.cache"), true},     // Project pattern
	}
	
	for _, tt := range tests {
		shouldIgnore, err := m.ShouldIgnore(tt.path)
		if err != nil {
			t.Errorf("Error checking %s: %v", tt.path, err)
			continue
		}
		if shouldIgnore != tt.ignore {
			t.Errorf("Path %s: expected ignore=%v, got %v", tt.path, tt.ignore, shouldIgnore)
		}
	}
}

func TestMatcherCache(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create ignore file
	ignoreFile := filepath.Join(tmpDir, ".dropboxignore")
	if err := os.WriteFile(ignoreFile, []byte("*.log"), 0644); err != nil {
		t.Fatalf("Failed to create ignore file: %v", err)
	}
	
	m, err := NewMatcher(2) // Small cache for testing
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	
	// First check should load from file
	testFile := filepath.Join(tmpDir, "test.log")
	shouldIgnore1, _ := m.ShouldIgnore(testFile)
	if !shouldIgnore1 {
		t.Error("Expected test.log to be ignored")
	}
	
	// Second check should use cache
	shouldIgnore2, _ := m.ShouldIgnore(testFile)
	if !shouldIgnore2 {
		t.Error("Expected test.log to be ignored (cached)")
	}
	
	// Clear cache and check again
	m.ClearCache()
	shouldIgnore3, _ := m.ShouldIgnore(testFile)
	if !shouldIgnore3 {
		t.Error("Expected test.log to be ignored after cache clear")
	}
}

func TestMatcherNoIgnoreFile(t *testing.T) {
	tmpDir := t.TempDir()
	
	m, err := NewMatcher(10)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	
	// Test file in directory without .dropboxignore
	testFile := filepath.Join(tmpDir, "test.txt")
	shouldIgnore, err := m.ShouldIgnore(testFile)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if shouldIgnore {
		t.Error("File should not be ignored when no .dropboxignore exists")
	}
}

func TestLoadIgnoreFile(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create ignore file with various patterns
	ignoreContent := `# Comment line
   
*.log
  # Another comment
/absolute/path
relative/path
!important.txt
  
# Empty lines above
`
	ignoreFile := filepath.Join(tmpDir, ".dropboxignore")
	if err := os.WriteFile(ignoreFile, []byte(ignoreContent), 0644); err != nil {
		t.Fatalf("Failed to create ignore file: %v", err)
	}
	
	m, err := NewMatcher(10)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	
	ignore, err := m.LoadIgnoreFile(ignoreFile)
	if err != nil {
		t.Fatalf("Failed to load ignore file: %v", err)
	}
	
	// Test pattern matching
	tests := []struct {
		path   string
		ignore bool
	}{
		{"test.log", true},
		{"dir/test.log", true},
		{"important.txt", false},
		{"absolute/path", true},
		{"relative/path", true},
	}
	
	for _, tt := range tests {
		if ignore.MatchesPath(tt.path) != tt.ignore {
			t.Errorf("Path %s: expected ignore=%v", tt.path, tt.ignore)
		}
	}
}