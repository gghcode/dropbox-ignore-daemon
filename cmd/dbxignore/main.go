package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	cli "github.com/urfave/cli/v3"
	
	"github.com/gghcode/dropbox-ignore-daemon/internal/matcher"
	"github.com/gghcode/dropbox-ignore-daemon/internal/poller"
	"github.com/gghcode/dropbox-ignore-daemon/internal/state"
	"github.com/gghcode/dropbox-ignore-daemon/internal/watcher"
	"github.com/gghcode/dropbox-ignore-daemon/internal/xattr"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// commonFlags defines flags shared between commands
var commonFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "root",
		Aliases: []string{"r"},
		Usage:   "Root directory to monitor/scan",
		Value:   "~/Dropbox",
	},
	&cli.BoolFlag{
		Name:    "dry-run",
		Aliases: []string{"n"},
		Usage:   "Show what would be done without making changes",
	},
	&cli.BoolFlag{
		Name:    "verbose",
		Aliases: []string{"v"},
		Usage:   "Enable verbose logging",
	},
}

func main() {
	// Performance optimizations
	runtime.GOMAXPROCS(1)
	debug.SetGCPercent(400)
	
	app := &cli.Command{
		Name:    "dbxignore",
		Usage:   "Dropbox ignore daemon",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, Date),
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "Run the daemon",
				Flags: append(commonFlags,
					&cli.DurationFlag{
						Name:  "scan-interval",
						Usage: "Polling interval",
						Value: 5 * time.Minute,
					},
				),
				Action: serve,
			},
			{
				Name:   "scan",
				Usage:  "Run a one-time scan",
				Flags:  commonFlags,
				Action: scan,
			},
			{
				Name:   "install",
				Usage:  "Install system service",
				Flags:  commonFlags[:1], // Only root flag
				Action: install,
			},
			{
				Name:   "uninstall",
				Usage:  "Uninstall system service",
				Action: uninstall,
			},
		},
	}
	
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func serve(ctx context.Context, cmd *cli.Command) error {
	cfg := getConfig(cmd)
	
	// Create components
	m, err := matcher.NewMatcher(32)
	if err != nil {
		return fmt.Errorf("failed to create matcher: %w", err)
	}
	
	cache := state.NewCache(1 * time.Minute)
	// StartCleaner now returns a no-op function, so we don't need to defer it
	
	// Create handler without worker pool
	handler := createHandler(m, cache, cfg.dryRun, cfg.logger)
	
	// Create watcher
	w, err := watcher.NewWatcher(watcher.Config{
		Handler: func(event watcher.Event) error {
			info, err := os.Stat(event.Path)
			if err != nil {
				return nil // File might have been deleted
			}
			return handler(event.Path, info)
		},
		Logger: cfg.logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer w.Close()
	
	// Add watches
	if err := w.AddRecursive(cfg.root); err != nil {
		return fmt.Errorf("failed to add watches: %w", err)
	}
	
	// Create poller
	p, err := poller.NewPoller(poller.Config{
		Root:         cfg.root,
		ScanInterval: cmd.Duration("scan-interval"),
		Handler:      handler,
		Logger:       cfg.logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create poller: %w", err)
	}
	
	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	var wg sync.WaitGroup
	
	// Start watcher
	wg.Add(1)
	go func() {
		defer wg.Done()
		cfg.logger.Printf("Starting filesystem watcher for %s", cfg.root)
		if err := w.Run(ctx); err != nil && err != context.Canceled {
			cfg.logger.Printf("Watcher error: %v", err)
		}
	}()
	
	// Start poller
	wg.Add(1)
	go func() {
		defer wg.Done()
		cfg.logger.Printf("Starting periodic scanner (interval: %v)", cmd.Duration("scan-interval"))
		if err := p.Run(ctx); err != nil && err != context.Canceled {
			cfg.logger.Printf("Poller error: %v", err)
		}
	}()
	
	// Wait for signal
	cfg.logger.Printf("Dropbox ignore daemon started. Press Ctrl+C to stop.")
	select {
	case <-sigCh:
		cfg.logger.Println("Shutting down...")
		cancel()
	case <-ctx.Done():
	}
	
	// Wait for goroutines
	wg.Wait()
	return nil
}

func scan(ctx context.Context, cmd *cli.Command) error {
	cfg := getConfig(cmd)
	
	// Create components
	m, err := matcher.NewMatcher(32)
	if err != nil {
		return fmt.Errorf("failed to create matcher: %w", err)
	}
	
	cache := state.NewCache(1 * time.Minute)
	
	// Create handler without worker pool
	handler := createHandler(m, cache, cfg.dryRun, cfg.logger)
	
	// Create poller for one-time scan
	p, err := poller.NewPoller(poller.Config{
		Root:    cfg.root,
		Handler: handler,
		Logger:  cfg.logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}
	
	cfg.logger.Printf("Scanning %s", cfg.root)
	return p.Scan()
}

func install(ctx context.Context, cmd *cli.Command) error {
	// TODO: Implement service installation
	return fmt.Errorf("install command not yet implemented")
}

func uninstall(ctx context.Context, cmd *cli.Command) error {
	// TODO: Implement service uninstallation
	return fmt.Errorf("uninstall command not yet implemented")
}

// config holds common configuration extracted from CLI flags
type config struct {
	root   string
	dryRun bool
	logger *log.Logger
}

// getConfig extracts common configuration from CLI command
func getConfig(cmd *cli.Command) config {
	return config{
		root:   expandPath(cmd.String("root")),
		dryRun: cmd.Bool("dry-run"),
		logger: setupLogger(cmd.Bool("verbose")),
	}
}

// createHandler creates a synchronous handler without worker pool
func createHandler(m *matcher.Matcher, cache *state.Cache, dryRun bool, logger *log.Logger) func(string, fs.FileInfo) error {
	return func(path string, info fs.FileInfo) error {
		// Check cache
		if cache.Has(path, info) {
			return nil
		}
		
		// Check if should ignore
		shouldIgnore, err := m.ShouldIgnore(path)
		if err != nil {
			logger.Printf("Matcher error for %s: %v", path, err)
			return nil // Continue processing other files
		}
		
		if !shouldIgnore {
			return nil
		}
		
		// Check if already ignored
		ignored, err := xattr.IsIgnored(path)
		if err != nil {
			logger.Printf("Failed to check xattr for %s: %v", path, err)
			return nil // Continue processing other files
		}
		
		if ignored {
			cache.Add(path, info)
			// If it's an ignored directory, skip its contents
			if info.IsDir() {
				return poller.ErrSkipDir
			}
			return nil
		}
		
		// Set ignore attribute
		if dryRun {
			logger.Printf("[DRY RUN] Would set ignore attribute on: %s", path)
		} else {
			if err := xattr.SetIgnored(path); err != nil {
				logger.Printf("Failed to set xattr for %s: %v", path, err)
				return nil // Continue processing other files
			}
			logger.Printf("Set ignore attribute on: %s", path)
		}
		
		cache.Add(path, info)
		
		// If we just set ignore on a directory, skip its contents
		if info.IsDir() {
			return poller.ErrSkipDir
		}
		return nil
	}
}

func setupLogger(verbose bool) *log.Logger {
	if verbose {
		return log.New(os.Stdout, "", log.LstdFlags)
	}
	return log.New(os.Stdout, "", 0)
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	abs, _ := filepath.Abs(path)
	return abs
}