// Package poller provides periodic filesystem scanning with incremental updates
package poller

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// Default scan interval
	defaultScanInterval = 5 * time.Minute
	// Maximum age for directory entries before cleanup
	maxDirAge = 2 * defaultScanInterval
)

// ErrSkipDir is returned by Handler to indicate that directory contents should be skipped
var ErrSkipDir = errors.New("skip directory")

// Handler is called for each file that needs processing
type Handler func(path string, info fs.FileInfo) error

// Poller performs periodic filesystem scans
type Poller struct {
	root     string
	interval time.Duration
	handler  Handler
	
	mu         sync.RWMutex
	lastScan   time.Time
	dirModTime map[string]time.Time
	
	logger *log.Logger
	
	// Directories to skip
	skipDirs map[string]bool
}

// Config holds poller configuration
type Config struct {
	Root         string
	ScanInterval time.Duration
	Handler      Handler
	Logger       *log.Logger
	SkipDirs     []string
}

// NewPoller creates a new filesystem poller
func NewPoller(cfg Config) (*Poller, error) {
	if cfg.ScanInterval <= 0 {
		cfg.ScanInterval = defaultScanInterval
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	
	// Resolve root to absolute path
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	
	// Default skip directories
	skipDirs := map[string]bool{
		".git":           true,
		".dropbox.cache": true,
		"node_modules":   true,
		".svn":           true,
		".hg":            true,
	}
	
	// Add custom skip directories
	for _, dir := range cfg.SkipDirs {
		skipDirs[dir] = true
	}
	
	return &Poller{
		root:       root,
		interval:   cfg.ScanInterval,
		handler:    cfg.Handler,
		dirModTime: make(map[string]time.Time),
		logger:     cfg.Logger,
		skipDirs:   skipDirs,
	}, nil
}

// Run starts the polling loop
func (p *Poller) Run(ctx context.Context) error {
	// Initial scan
	if err := p.Scan(); err != nil {
		p.logger.Printf("Initial scan error: %v", err)
	}
	
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.Scan(); err != nil {
				p.logger.Printf("Scan error: %v", err)
			}
		}
	}
}

// Scan performs a filesystem scan
func (p *Poller) Scan() error {
	scanStart := time.Now()
	
	// Update last scan time
	p.mu.Lock()
	lastScan := p.lastScan
	p.lastScan = scanStart
	p.mu.Unlock()
	
	p.logger.Printf("Starting scan of %s", p.root)
	
	fileCount := 0
	dirCount := 0
	skippedDirs := 0
	
	// Track visited directories for cleanup
	visitedDirs := make(map[string]struct{})
	
	err := filepath.WalkDir(p.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Log but continue scanning
			p.logger.Printf("Walk error at %s: %v", path, err)
			return nil
		}
		
		// Check if directory should be skipped
		if d.IsDir() {
			dirCount++
			visitedDirs[path] = struct{}{}
			
			// Get directory info first
			info, err := d.Info()
			if err != nil {
				p.logger.Printf("Failed to get info for directory %s: %v", path, err)
				return nil
			}
			
			// Process directory first (for applying ignore attributes)
			err = p.handler(path, info)
			if err != nil {
				if errors.Is(err, ErrSkipDir) {
					p.logger.Printf("Skipping contents of ignored directory: %s", path)
					skippedDirs++
					return filepath.SkipDir
				}
				p.logger.Printf("Handler error for directory %s: %v", path, err)
			}
			
			// Now check if we should skip descending into this directory
			name := d.Name()
			if p.skipDirs[name] || strings.HasPrefix(name, ".") && name != "." {
				skippedDirs++
				return filepath.SkipDir
			}
			
			// Check directory modification time for incremental scan
			if !lastScan.IsZero() {
				// Check if directory can be skipped
				if p.shouldSkipDir(path, info.ModTime()) {
					return filepath.SkipDir
				}
				// Update modification time
				p.updateDirModTime(path, info.ModTime())
			}
		} else {
			fileCount++
			// Get file info
			info, err := d.Info()
			if err != nil {
				p.logger.Printf("Failed to get info for %s: %v", path, err)
				return nil
			}
			
			// Skip if file hasn't been modified since last scan
			if !lastScan.IsZero() && info.ModTime().Before(lastScan) {
				return nil
			}
			
			// Process file
			if err := p.handler(path, info); err != nil {
				p.logger.Printf("Handler error for %s: %v", path, err)
			}
		}
		
		return nil
	})
	
	// Clean up old directory entries
	p.cleanupDirModTime(visitedDirs)
	
	duration := time.Since(scanStart)
	p.logger.Printf("Scan completed in %v: %d files, %d dirs (%d skipped)", 
		duration, fileCount, dirCount, skippedDirs)
	
	return err
}

// shouldSkipDir checks if a directory can be skipped based on modification time
func (p *Poller) shouldSkipDir(path string, modTime time.Time) bool {
	p.mu.RLock()
	prevModTime, exists := p.dirModTime[path]
	p.mu.RUnlock()
	
	return exists && modTime.Equal(prevModTime)
}

// updateDirModTime updates the modification time for a directory
func (p *Poller) updateDirModTime(path string, modTime time.Time) {
	p.mu.Lock()
	p.dirModTime[path] = modTime
	p.mu.Unlock()
}

// cleanupDirModTime removes entries for directories that no longer exist
func (p *Poller) cleanupDirModTime(visitedDirs map[string]struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Remove entries for directories that weren't visited
	for path := range p.dirModTime {
		if _, visited := visitedDirs[path]; !visited {
			delete(p.dirModTime, path)
		}
	}
}

// SetSkipDir adds or removes a directory from the skip list
func (p *Poller) SetSkipDir(name string, skip bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if skip {
		p.skipDirs[name] = true
	} else {
		delete(p.skipDirs, name)
	}
}

// GetLastScan returns the time of the last scan
func (p *Poller) GetLastScan() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastScan
}

// ClearCache clears the directory modification time cache
func (p *Poller) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dirModTime = make(map[string]time.Time)
}