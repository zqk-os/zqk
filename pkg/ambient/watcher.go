package ambient

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// WatcherConfig defines configuration for filesystem ambient monitoring.
type WatcherConfig struct {
	// IgnoredDirs is a list of directory names to skip during recursive directory walking.
	IgnoredDirs []string `json:"ignored_dirs" yaml:"ignored_dirs"`
	// IgnoredPaths is a list of path substrings or patterns to ignore for events and watching.
	IgnoredPaths []string `json:"ignored_paths" yaml:"ignored_paths"`
}

// DefaultIgnoredDirNames returns the generic, universal directory names that are always ignored.
// It includes repository source trees, documentation, and build artifacts to prevent Darwin kqueue descriptor exhaustion.
func DefaultIgnoredDirNames() []string {
	return []string{
		".git",
		".svn",
		".hg",
		".idea",
		".vscode",
		".agent",
		".agents",
		".cursor",
		".ide",
		".github",
		"node_modules",
		"vendor",
		paths.ProjectDataDir,
		paths.DefaultProjectStateDir,
		".tmp",
		".cache",
		"coverage",
		"pkg",
		"cmd",
		"internal",
		"bin",
		"tools",
		"scripts",
		"packs",
		"dist",
		"dist-docs",
		"dist-community",
		"ext",
		"docs",
		"examples",
		"config",
	}
}

// DefaultIgnoredPaths returns universal path patterns to ignore to prevent recursive event loops and descriptor leaks.
func DefaultIgnoredPaths() []string {
	return []string{
		"/.git/",
		"/.git",
		"/.svn/",
		"/.hg/",
		"/.idea/",
		"/.vscode/",
		"/.agent/",
		"/.agent",
		"/.agents/",
		"/.agents",
		"/.cursor/",
		"/.cursor",
		"/.ide/",
		"/.ide",
		"/.github/",
		"/.github",
		"/node_modules/",
		"/vendor/",
		"/.cache/",
		"/.tmp/",
		"/coverage/",
		"/.DS_Store",
		"/" + paths.ProjectDataDir + "/",
		"/" + paths.ProjectDataDir,
		"/" + paths.DefaultProjectStateDir + "/",
		"/" + paths.DefaultProjectStateDir,
		"/pkg/",
		"/cmd/",
		"/internal/",
		"/bin/",
		"/tools/",
		"/scripts/",
		"/packs/",
		"/dist/",
		"/dist-",
		"/ext/",
		"/docs/",
		"/docs",
		"/examples/",
		"/examples",
		"/config/",
		"/config",
	}
}

// DefaultWatcherConfig returns a WatcherConfig populated with universal defaults.
func DefaultWatcherConfig() WatcherConfig {
	return WatcherConfig{
		IgnoredDirs:  DefaultIgnoredDirNames(),
		IgnoredPaths: DefaultIgnoredPaths(),
	}
}

type ambientConfigYAML struct {
	IgnoredDirs  []string `yaml:"ignored_dirs"`
	IgnoredPaths []string `yaml:"ignored_paths"`
	Watcher      struct {
		IgnoredDirs  []string `yaml:"ignored_dirs"`
		IgnoredPaths []string `yaml:"ignored_paths"`
	} `yaml:"watcher"`
}

// LoadWatcherConfig loads watcher configuration starting with universal defaults,
// layered with .zqk/ambient.yaml (or .ambientignore) and environment overrides.
func LoadWatcherConfig(rootPath string) WatcherConfig {
	cfg := DefaultWatcherConfig()

	// 1. Try .zqk/ambient.yaml or .zqk/ambient.yml
	ambientYAMLPaths := []string{
		filepath.Join(rootPath, paths.ProjectDataDir, "ambient.yaml"),
		filepath.Join(rootPath, paths.ProjectDataDir, "ambient.yml"),
	}
	for _, p := range ambientYAMLPaths {
		if data, err := fileutil.ReadFile(p); err == nil && len(data) > 0 {
			var parsed ambientConfigYAML
			if err := yaml.Unmarshal(data, &parsed); err == nil {
				cfg.IgnoredDirs = append(cfg.IgnoredDirs, parsed.IgnoredDirs...)
				cfg.IgnoredDirs = append(cfg.IgnoredDirs, parsed.Watcher.IgnoredDirs...)
				cfg.IgnoredPaths = append(cfg.IgnoredPaths, parsed.IgnoredPaths...)
				cfg.IgnoredPaths = append(cfg.IgnoredPaths, parsed.Watcher.IgnoredPaths...)
			}
			break
		}
	}

	// 2. Try .ambientignore in rootPath
	ambientIgnorePath := filepath.Join(rootPath, ".ambientignore")
	if data, err := fileutil.ReadFile(ambientIgnorePath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.Contains(line, "/") {
				cfg.IgnoredPaths = append(cfg.IgnoredPaths, line)
			} else {
				cfg.IgnoredDirs = append(cfg.IgnoredDirs, line)
			}
		}
	}

	// 3. Environment overrides
	if envDirs := zqkenv.AmbientIgnoreDirs().Get(); envDirs != "" {
		for _, d := range strings.Split(envDirs, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				cfg.IgnoredDirs = append(cfg.IgnoredDirs, d)
			}
		}
	}
	if envPaths := zqkenv.AmbientIgnorePaths().Get(); envPaths != "" {
		for _, p := range strings.Split(envPaths, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.IgnoredPaths = append(cfg.IgnoredPaths, p)
			}
		}
	}

	return cfg
}

type FSWatcher struct {
	rootPath     string
	eventHub     EventHub
	watcher      *fsnotify.Watcher
	done         chan struct{}
	config       WatcherConfig
	ignoredDirs  map[string]bool
	ignoredPaths []string
}

// NewFSWatcherWithConfig creates a new FSWatcher instance with explicit configuration.
func NewFSWatcherWithConfig(rootPath string, eventHub EventHub, cfg WatcherConfig) (*FSWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	dirMap := make(map[string]bool)
	for _, d := range cfg.IgnoredDirs {
		d = strings.TrimSpace(d)
		if d != "" {
			dirMap[d] = true
		}
	}
	var cleanPaths []string
	for _, p := range cfg.IgnoredPaths {
		p = strings.TrimSpace(p)
		if p != "" {
			cleanPaths = append(cleanPaths, filepath.ToSlash(p))
		}
	}
	return &FSWatcher{
		rootPath:     rootPath,
		eventHub:     eventHub,
		watcher:      watcher,
		done:         make(chan struct{}),
		config:       cfg,
		ignoredDirs:  dirMap,
		ignoredPaths: cleanPaths,
	}, nil
}

// NewFSWatcher creates a new FSWatcher instance, automatically resolving configuration
// from .zqk/ambient.yaml, .ambientignore, or environment overrides.
func NewFSWatcher(rootPath string, eventHub EventHub) (*FSWatcher, error) {
	cfg := LoadWatcherConfig(rootPath)
	return NewFSWatcherWithConfig(rootPath, eventHub, cfg)
}

func (fw *FSWatcher) isIgnoredDirName(name string) bool {
	if strings.HasPrefix(name, ".tmp") || strings.HasPrefix(name, "dist") {
		return true
	}
	return fw.ignoredDirs[name]
}

func (fw *FSWatcher) isIgnoredFSPath(path string) bool {
	clean := filepath.ToSlash(path)
	for _, p := range fw.ignoredPaths {
		if strings.Contains(clean, p) || strings.HasSuffix(clean, p) {
			return true
		}
	}
	return false
}

// isIgnoredDirName checks if a directory name matches universal default ignores (package-level compatibility helper).
func isIgnoredDirName(name string) bool {
	if strings.HasPrefix(name, ".tmp") || strings.HasPrefix(name, "dist") {
		return true
	}
	for _, d := range DefaultIgnoredDirNames() {
		if d == name {
			return true
		}
	}
	return false
}

// isIgnoredFSPath checks if a path matches universal default ignores (package-level compatibility helper).
func isIgnoredFSPath(path string) bool {
	clean := filepath.ToSlash(path)
	for _, p := range DefaultIgnoredPaths() {
		if strings.Contains(clean, p) || strings.HasSuffix(clean, p) {
			return true
		}
	}
	return false
}

// Start begins watching the directory tree.
func (fw *FSWatcher) Start(ctx context.Context) error {
	// Walk the root directory and add all directories to watcher, ignoring noise and configured ignores
	err := filepath.Walk(fw.rootPath, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if fw.isIgnoredDirName(base) || fw.isIgnoredFSPath(path) {
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
					if fw.isIgnoredFSPath(event.Name) {
						continue
					}

					// If it's a new directory, add it to watcher
					info, err := fileutil.Stat(event.Name)
					if err == nil && info.IsDir() {
						base := filepath.Base(event.Name)
						if !fw.isIgnoredDirName(base) && !fw.isIgnoredFSPath(event.Name) {
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
