package utility

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

const emptyValue = ""

const (
	configWatcherChangeDeleted  = "deleted"
	configWatcherChangeCreated  = "created"
	configWatcherChangeModified = "modified"
	configWatcherEventKey       = "event"
)

// ConfigFileWatcher monitors configuration files for changes and emits events
// when files are modified. This enables event-driven reloads of configuration
// trees/forests based on file change events.
type ConfigFileWatcher struct {
	projectRoot  string
	internalDir  string
	coordinator  coordination.EventCoordinator
	logger       logging.Logger
	watchedFiles map[string]fileState // filename -> last known state
	mu           sync.RWMutex
	stopChan     chan struct{}
	watching     bool
	pollInterval time.Duration
}

// fileState tracks the state of a watched file
type fileState struct {
	Path      string
	MTime     time.Time
	Size      int64
	ReloadFn  func() error // Function to trigger reload when file changes
	EventType string       // Event type to emit (e.g., "config_id_prefixes_changed")
}

// NewConfigFileWatcher creates a new config file watcher
func NewConfigFileWatcher(projectRoot string, coordinator coordination.EventCoordinator, logger logging.Logger) *ConfigFileWatcher {
	internalDir := filepath.Join(projectRoot, paths.ProcessInternalDir)
	return &ConfigFileWatcher{
		projectRoot:  projectRoot,
		internalDir:  internalDir,
		coordinator:  coordinator,
		logger:       logger,
		watchedFiles: make(map[string]fileState),
		stopChan:     make(chan struct{}),
		pollInterval: 5 * time.Second, // Poll every 5 seconds (can be made configurable)
	}
}

// RegisterFile registers a config file to watch
// reloadFn is called when the file changes to trigger appropriate reloads
func (w *ConfigFileWatcher) RegisterFile(filename, eventType string, reloadFn func() error) {
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&w.mu,
		pkgctx.NewSystemContext(),
		nil,
		logger,
		LockNameConfigWatcherRegisterFile,
		func() error {
			filePath := filepath.Join(w.internalDir, filename)
			w.watchedFiles[filename] = fileState{
				Path:      filePath,
				ReloadFn:  reloadFn,
				EventType: eventType,
			}

			// Initialize state from current file if it exists
			if info, err := fileutil.Stat(filePath); err == nil {
				w.watchedFiles[filename] = fileState{
					Path:      filePath,
					MTime:     info.ModTime(),
					Size:      info.Size(),
					ReloadFn:  reloadFn,
					EventType: eventType,
				}
			}
			return nil
		},
	)
}

// Start begins watching for file changes (non-blocking)
func (w *ConfigFileWatcher) Start(ctx context.Context) error {
	var alreadyWatching bool
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	err := concurrency.WithLockTimeout(
		&w.mu,
		ctx,
		nil,
		logger,
		LockNameConfigWatcherStartCheck,
		func() error {
			if w.watching {
				alreadyWatching = true
				return nil
			}
			w.watching = true
			return nil
		},
	)
	if err != nil {
		return err
	}

	if alreadyWatching {
		return errfmt.Errorf("watcher is already running")
	}

	// Start watching in background goroutine
	goroutinelabels.NewGoroutine("config_watcher.loop", "watching config file changes").StartSimple(func() {
		w.watchLoop(ctx)
	})

	return nil
}

// Stop stops watching for file changes
func (w *ConfigFileWatcher) Stop() {
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&w.mu,
		pkgctx.NewSystemContext(),
		nil,
		logger,
		LockNameConfigWatcherStop,
		func() error {
			if !w.watching {
				return nil
			}

			close(w.stopChan)
			w.watching = false
			return nil
		},
	)
}

// watchLoop polls files for changes and emits events
func (w *ConfigFileWatcher) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.checkForChanges(ctx)
		}
	}
}

// checkForChanges checks all watched files for changes and emits events
func (w *ConfigFileWatcher) checkForChanges(ctx context.Context) {
	var files map[string]fileState
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&w.mu,
		ctx,
		nil,
		logger,
		LockNameConfigWatcherCheckCopy,
		func() error {
			files = make(map[string]fileState, len(w.watchedFiles))
			for k, v := range w.watchedFiles {
				files[k] = v
			}
			return nil
		},
	)

	for filename, state := range files {
		info, err := fileutil.Stat(state.Path)
		if err != nil {
			if fileutil.IsNotExist(err) {
				// File was deleted - emit event
				w.emitFileChangeEvent(ctx, filename, state, nil, configWatcherChangeDeleted)
				// Update state to reflect deletion
				_ = concurrency.WithLockTimeout(
					&w.mu,
					ctx,
					nil,
					logger,
					LockNameConfigWatcherUpdateDeleted,
					func() error {
						if currentState, exists := w.watchedFiles[filename]; exists {
							currentState.MTime = time.Time{}
							currentState.Size = 0
							w.watchedFiles[filename] = currentState
						}
						return nil
					},
				)
			}
			continue
		}

		// Check if file changed (mtime or size)
		changed := false
		if state.MTime.IsZero() {
			// File was created (wasn't there before)
			changed = true
		} else if !info.ModTime().Equal(state.MTime) || info.Size() != state.Size {
			// File was modified
			changed = true
		}

		if changed {
			oldState := state
			newState := fileState{
				Path:      state.Path,
				MTime:     info.ModTime(),
				Size:      info.Size(),
				ReloadFn:  state.ReloadFn,
				EventType: state.EventType,
			}

			// Emit event
			eventType := configWatcherChangeCreated
			if !oldState.MTime.IsZero() {
				eventType = configWatcherChangeModified
			}
			w.emitFileChangeEvent(ctx, filename, oldState, &newState, eventType)

			// Trigger reload handler
			if state.ReloadFn != nil {
				if err := state.ReloadFn(); err != nil {
					logging.Fluent(w.logger).Warn("Failed to trigger config reload after file change").
						File(filename).
						WithError(err).
						Log()
				} else {
					logging.Fluent(w.logger).Debug("Triggered config reload").
						File(filename).
						EventType(state.EventType).
						Log()
				}
			}

			// Update state
			_ = concurrency.WithLockTimeout(
				&w.mu,
				ctx,
				nil,
				logger,
				LockNameConfigWatcherCheckUpdate,
				func() error {
					if currentState, exists := w.watchedFiles[filename]; exists {
						currentState.MTime = info.ModTime()
						currentState.Size = info.Size()
						w.watchedFiles[filename] = currentState
					}
					return nil
				},
			)
		}
	}
}

// emitFileChangeEvent emits a file change event via the coordinator
func (w *ConfigFileWatcher) emitFileChangeEvent(ctx context.Context, filename string, oldState fileState, newState *fileState, changeType string) {
	if w.coordinator == nil {
		return
	}

	fields := map[string]any{
		"file":                     filename,
		objects.FieldKeyPath:       oldState.Path,
		objects.FieldKeyChangeType: changeType,
	}

	if !oldState.MTime.IsZero() {
		fields["old_mtime"] = oldState.MTime
		fields["old_size"] = oldState.Size
	}

	if newState != nil {
		fields["new_mtime"] = newState.MTime
		fields["new_size"] = newState.Size
	}

	// Create event context
	eventData := &coordination.EventData{
		LoggingFields: []coordination.LoggingField{
			{Key: configWatcherEventKey, Value: fmt.Sprintf("Config file %s %s", filename, changeType)},
			{Key: "file", Value: filename},
			{Key: "change_type", Value: changeType},
		},
	}

	eventCtx := coordination.NewEventContext(
		fmt.Sprintf("config-watcher-%d", time.Now().UnixNano()),
		oldState.EventType, // Use the event type registered for this file
		changeType,
	).WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, true) // Logging and Operational channels

	_ = w.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Best effort
}

// SetupDefaultWatchers registers default config files to watch
func (w *ConfigFileWatcher) SetupDefaultWatchers() {
	// Register ID prefixes config
	w.RegisterFile("id_prefixes_config.yaml", "config_id_prefixes_changed", func() error {
		validation.ResetGlobalIDPrefixesConfig()
		return nil
	})

	// Register paths config
	w.RegisterFile("paths_config.yaml", "config_paths_changed", func() error {
		validation.ResetGlobalPathsConfig()
		return nil
	})

	w.RegisterFile("kind_mappings_config.yaml", "config_kind_mappings_changed", func() error {
		objects.ResetGlobalKindMappingsConfig()
		return nil
	})

	w.RegisterFile("namespaces_config.yaml", "config_namespaces_changed", func() error {
		validation.ResetGlobalNamespacesConfig()
		return nil
	})
}
