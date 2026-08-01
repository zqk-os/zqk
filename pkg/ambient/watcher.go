package ambient

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/fsnotify/fsnotify"
	"github.com/lanceman/zqk/pkg/objects"
)

// FSWatcher wraps fsnotify to provide a filtered, high-signal event stream.
type FSWatcher struct {
	watcher      *fsnotify.Watcher
	eventHub     EventHub
	analyzer     *ASTAnalyzer
	rootPath     string
	ignoredDirs  map[string]bool
	ignoredExts  map[string]bool
	done         chan struct{}
	mu           sync.Mutex
	isWatching   bool
	debounceMap  map[string]time.Time
	debounceLock sync.Mutex
}

// NewFSWatcher creates a new FSWatcher instance.
func NewFSWatcher(rootPath string, eventHub EventHub) (*FSWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %v", err)
	}

	return &FSWatcher{
		watcher:  w,
		eventHub: eventHub,
		analyzer: NewASTAnalyzer(),
		rootPath: rootPath,
		ignoredDirs: map[string]bool{
			".git":               true,
			"node_modules":       true,
			paths.ProjectDataDir: true,
			"vendor":             true,
			"bin":                true,
			"dist":               true,
		},
		ignoredExts: map[string]bool{
			".swp":      true,
			".tmp":      true,
			".DS_Store": true,
		},
		done:        make(chan struct{}),
		debounceMap: make(map[string]time.Time),
	}, nil
}

// Start begins watching the directory tree.
func (fw *FSWatcher) Start(ctx context.Context) error {
	fw.mu.Lock()
	if fw.isWatching {
		fw.mu.Unlock()
		return fmt.Errorf("FSWatcher is already running")
	}
	fw.isWatching = true
	fw.mu.Unlock()

	// Walk root path and add directories to watcher
	err := filepath.Walk(fw.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // ignore permission errors during walk
		}
		if info.IsDir() {
			if fw.isIgnoredDir(path) {
				return filepath.SkipDir
			}
			err = fw.watcher.Add(path)
			if err != nil {
				log.Printf("[FSWatcher] failed to watch dir %s: %v", path, err)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk root path: %v", err)
	}

	go fw.loop(ctx)
	return nil
}

// Stop halts the watcher.
func (fw *FSWatcher) Stop() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if !fw.isWatching {
		return nil
	}

	fw.isWatching = false
	close(fw.done)
	return fw.watcher.Close()
}

func (fw *FSWatcher) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			_ = fw.Stop()
			return
		case <-fw.done:
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// Ignore chmod/rename noise, focus on creation/writes
			if event.Has(fsnotify.Chmod) || event.Has(fsnotify.Rename) {
				continue
			}

			if fw.isIgnoredFile(event.Name) {
				continue
			}

			// Debounce rapid writes
			fw.debounceLock.Lock()
			lastEvent, exists := fw.debounceMap[event.Name]
			now := time.Now()
			if exists && now.Sub(lastEvent) < 500*time.Millisecond {
				fw.debounceLock.Unlock()
				continue
			}
			fw.debounceMap[event.Name] = now
			fw.debounceLock.Unlock()

			// Emit to the Ambient Event Hub
			_ = fw.eventHub.Publish(ctx, Event{
				Type: EventTypeFilesystem,
				Payload: map[string]any{
					objects.FieldKeySource:    "fswatcher",
					objects.FieldKeyTargetID:  event.Name,
					objects.FieldKeyOperation: event.Op.String(),
				},
				Timestamp: time.Now(),
			})

			// Run the AST Predictor on Go files
			if strings.HasSuffix(event.Name, ".go") && (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) {
				preds, err := fw.analyzer.Analyze(event.Name)
				if err == nil && len(preds) > 0 {
					_ = fw.eventHub.Publish(ctx, Event{
						Type: EventTypeFilesystem,
						Payload: map[string]any{
							objects.FieldKeySource:      "ast_analyzer",
							objects.FieldKeyPredictions: preds,
						},
						Timestamp: time.Now(),
					})
				}
			}

			// Automatically add new directories to the watcher
			if event.Has(fsnotify.Create) {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() && !fw.isIgnoredDir(event.Name) {
					_ = fw.watcher.Add(event.Name)
				}
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[FSWatcher] error: %v\n", err)
		}
	}
}

func (fw *FSWatcher) isIgnoredDir(path string) bool {
	// Strip root path for relative checking
	relPath := strings.TrimPrefix(path, fw.rootPath)
	relPath = strings.TrimPrefix(relPath, "/")

	parts := strings.Split(relPath, string(os.PathSeparator))
	for _, part := range parts {
		if fw.ignoredDirs[part] {
			return true
		}
	}
	return false
}

func (fw *FSWatcher) isIgnoredFile(path string) bool {
	if fw.isIgnoredDir(filepath.Dir(path)) {
		return true
	}
	ext := filepath.Ext(path)
	return fw.ignoredExts[ext]
}
