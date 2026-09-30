package ambient

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type FSWatcher struct {
	rootPath string
	eventHub EventHub
	watcher  *fsnotify.Watcher
	done     chan struct{}
}

// NewFSWatcher creates a new FSWatcher instance.
func NewFSWatcher(rootPath string, eventHub EventHub) (*FSWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &FSWatcher{
		rootPath: rootPath,
		eventHub: eventHub,
		watcher:  watcher,
		done:     make(chan struct{}),
	}, nil
}

// isIgnoredDirName returns true if a directory name should never be walked or watched.
func isIgnoredDirName(name string) bool {
	if strings.HasPrefix(name, "dist") || strings.HasPrefix(name, ".tmp") {
		return true
	}
	switch name {
	case ".git", "node_modules", ".idea", ".vscode", "vendor", paths.ProjectDataDir, paths.DefaultProjectStateDir, ".tmp",
		"pkg", "cmd", "internal", "bin", "tools", "scripts", "packs", "ext", ".cache", "coverage":
		return true
	default:
		return false
	}
}

// isIgnoredFSPath returns true if the given path belongs to internal state, runtime directories,
// or ignored trees that must never generate ambient events (prevents recursive write-event feedback loops and descriptor leaks).
func isIgnoredFSPath(path string) bool {
	clean := filepath.ToSlash(path)
	return strings.Contains(clean, "/.git/") ||
		strings.Contains(clean, "/"+paths.ProjectDataDir+"/") ||
		strings.Contains(clean, "/"+paths.DefaultProjectStateDir+"/") ||
		strings.Contains(clean, "/node_modules/") ||
		strings.Contains(clean, "/vendor/") ||
		strings.Contains(clean, "/.idea/") ||
		strings.Contains(clean, "/.vscode/") ||
		strings.Contains(clean, "/pkg/") ||
		strings.Contains(clean, "/cmd/") ||
		strings.Contains(clean, "/internal/") ||
		strings.Contains(clean, "/bin/") ||
		strings.Contains(clean, "/dist/") ||
		strings.Contains(clean, "/dist-") ||
		strings.Contains(clean, "/packs/") ||
		strings.Contains(clean, "/scripts/") ||
		strings.Contains(clean, "/tools/") ||
		strings.Contains(clean, "/ext/") ||
		strings.Contains(clean, "/.cache/") ||
		strings.Contains(clean, "/coverage/") ||
		strings.HasSuffix(clean, "/.DS_Store") ||
		strings.HasSuffix(clean, "/.git") ||
		strings.HasSuffix(clean, "/"+paths.ProjectDataDir) ||
		strings.HasSuffix(clean, "/"+paths.DefaultProjectStateDir)
}

// Start begins watching the directory tree.
func (fw *FSWatcher) Start(ctx context.Context) error {
	// Walk the root directory and add all directories to watcher, ignoring noise and internal kernel state
	err := filepath.Walk(fw.rootPath, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if isIgnoredDirName(base) || isIgnoredFSPath(path) {
				return filepath.SkipDir
			}
			return fw.watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	goroutinelabels.NewGoroutine("fswatcher", "listen to filesystem events").StartSimple(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-fw.done:
				return
			case event, ok := <-fw.watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					// Ignore events in ignored directories
					if isIgnoredFSPath(event.Name) {
						continue
					}

					// If it's a new directory, add it to watcher
					info, err := fileutil.Stat(event.Name)
					if err == nil && info.IsDir() {
						base := filepath.Base(event.Name)
						if !isIgnoredDirName(base) && !isIgnoredFSPath(event.Name) {
							_ = fw.watcher.Add(event.Name)
						}
						continue
					}

					// Publish event
					_ = fw.eventHub.Publish(context.Background(), Event{
						Type: EventTypeFilesystem,
						Payload: map[string]any{
							objects.FieldKeyTargetID:  event.Name,
							objects.FieldKeyOperation: event.Op.String(),
						},
						Timestamp: time.Now(),
					})
				}
			case <-fw.watcher.Errors:
				// just ignore for now
			}
		}
	})

	return nil
}

// Stop halts the watcher.
func (fw *FSWatcher) Stop() error {
	close(fw.done)
	return fw.watcher.Close()
}
