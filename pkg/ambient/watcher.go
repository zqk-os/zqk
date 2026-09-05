package ambient

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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

// Start begins watching the directory tree.
func (fw *FSWatcher) Start(ctx context.Context) error {
	// Walk the root directory and add all directories to watcher, ignoring noise
	err := filepath.Walk(fw.rootPath, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == ".idea" || base == ".vscode" || base == "vendor" {
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
					if strings.Contains(event.Name, "/.git/") || strings.Contains(event.Name, "/node_modules/") {
						continue
					}

					// If it's a new directory, add it to watcher
					info, err := fileutil.Stat(event.Name)
					if err == nil && info.IsDir() {
						base := filepath.Base(event.Name)
						if base != ".git" && base != "node_modules" && base != ".idea" && base != ".vscode" && base != "vendor" {
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
