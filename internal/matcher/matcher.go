// Package matcher provides glob pattern matching for .dropboxignore files
// with LRU caching for compiled patterns.
package matcher

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	gitignore "github.com/sabhiram/go-gitignore"
)

const (
	// Default cache size for compiled ignore patterns
	defaultCacheSize = 32
	// Name of the ignore file
	ignoreFileName = ".dropboxignore"
)

// Matcher manages ignore patterns from .dropboxignore files
type Matcher struct {
	mu    sync.RWMutex
	cache *lru.Cache[string, *gitignore.GitIgnore]
}

// NewMatcher creates a new pattern matcher with specified cache size
func NewMatcher(cacheSize int) (*Matcher, error) {
	if cacheSize <= 0 {
		cacheSize = defaultCacheSize
	}
	
	cache, err := lru.New[string, *gitignore.GitIgnore](cacheSize)
	if err != nil {
		return nil, err
	}
	
	return &Matcher{
		cache: cache,
	}, nil
}

// ShouldIgnore checks if a path should be ignored based on .dropboxignore patterns
func (m *Matcher) ShouldIgnore(path string) (bool, error) {
	// Find the closest .dropboxignore file
	dir := filepath.Dir(path)
	ignoreFile := m.findIgnoreFile(dir)
	if ignoreFile == "" {
		return false, nil
	}
	
	// Get or load the ignore patterns
	ignore, err := m.getOrLoadIgnore(ignoreFile)
	if err != nil {
		return false, err
	}
	
	// Check if path matches any pattern
	relPath, err := filepath.Rel(filepath.Dir(ignoreFile), path)
	if err != nil {
		return false, err
	}
	
	// For directories, also check with trailing slash
	matches := ignore.MatchesPath(relPath)
	if !matches {
		// Check if it's a directory
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			// Try matching with trailing slash
			matches = ignore.MatchesPath(relPath + "/")
		}
	}
	
	return matches, nil
}

// LoadIgnoreFile loads patterns from a .dropboxignore file
func (m *Matcher) LoadIgnoreFile(path string) (*gitignore.GitIgnore, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}
	
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	
	return gitignore.CompileIgnoreLines(patterns...), nil
}

// getOrLoadIgnore retrieves patterns from cache or loads from file
func (m *Matcher) getOrLoadIgnore(path string) (*gitignore.GitIgnore, error) {
	m.mu.RLock()
	if ignore, ok := m.cache.Get(path); ok {
		m.mu.RUnlock()
		return ignore, nil
	}
	m.mu.RUnlock()
	
	// Load from file
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Double-check after acquiring write lock
	if ignore, ok := m.cache.Get(path); ok {
		return ignore, nil
	}
	
	ignore, err := m.LoadIgnoreFile(path)
	if err != nil {
		return nil, err
	}
	
	m.cache.Add(path, ignore)
	return ignore, nil
}

// findIgnoreFile searches for the closest .dropboxignore file
func (m *Matcher) findIgnoreFile(dir string) string {
	for {
		ignoreFile := filepath.Join(dir, ignoreFileName)
		if _, err := os.Stat(ignoreFile); err == nil {
			return ignoreFile
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}
	return ""
}

// ClearCache removes all cached patterns
func (m *Matcher) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache.Purge()
}

// InvalidatePath removes cached patterns for a specific ignore file
func (m *Matcher) InvalidatePath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache.Remove(path)
}