package ambient

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objectidcache"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// WatcherConfig defines configuration for filesystem ambient monitoring.
type WatcherConfig struct {
	// WatchDirs is an optional list of specific directories or files to watch.
	// If empty, the root directory is watched (respecting IgnoredDirs and IgnoredPaths).
	WatchDirs []string `json:"watch_dirs" yaml:"watch_dirs"`
	// IgnoredDirs is a list of directory names to skip during recursive directory walking.
	IgnoredDirs []string `json:"ignored_dirs" yaml:"ignored_dirs"`
	// IgnoredPaths is a list of path substrings or patterns to ignore for events and watching.
	IgnoredPaths []string `json:"ignored_paths" yaml:"ignored_paths"`
}

// DefaultIgnoredDirNames returns the generic, universal toolchain and VCS directory names that are always ignored.
// Project-specific source, build, and documentation directories should be configured via config/ambient.yaml,
// .zqk/ambient.yaml, .ambientignore, or .gitignore.
func DefaultIgnoredDirNames() []string {
	return []string{
		".git",
		".svn",
		".hg",
		".idea",
		".vscode",
		"node_modules",
		"vendor",
		paths.ProjectDataDir,
		paths.DefaultProjectStateDir,
		".tmp",
		".cache",
		"coverage",
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
	WatchDirs    []string `yaml:"watch_dirs"`
	Watcher      struct {
		IgnoredDirs  []string `yaml:"ignored_dirs"`
		IgnoredPaths []string `yaml:"ignored_paths"`
		WatchDirs    []string `yaml:"watch_dirs"`
	} `yaml:"watcher"`
	Ambient struct {
		IgnoredDirs  []string `yaml:"ignored_dirs"`
		IgnoredPaths []string `yaml:"ignored_paths"`
		WatchDirs    []string `yaml:"watch_dirs"`
		Watcher      struct {
			IgnoredDirs  []string `yaml:"ignored_dirs"`
			IgnoredPaths []string `yaml:"ignored_paths"`
			WatchDirs    []string `yaml:"watch_dirs"`
		} `yaml:"watcher"`
	} `yaml:"ambient"`
}

func parseIgnoreFileLines(data []byte, cfg *WatcherConfig) {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "/") || strings.Contains(line, "*") {
			cfg.IgnoredPaths = append(cfg.IgnoredPaths, line)
		} else {
			cfg.IgnoredDirs = append(cfg.IgnoredDirs, line)
		}
	}
}

// LoadWatcherConfig loads watcher configuration starting with universal defaults,
// layered with config/ambient.yaml, .zqk/ambient.yaml, .ambientignore, .gitignore, and environment overrides.
func LoadWatcherConfig(rootPath string) WatcherConfig {
	cfg := DefaultWatcherConfig()

	// 1. Check YAML config candidates in priority order
	yamlCandidates := []string{
		filepath.Join(rootPath, "config", "ambient.yaml"),
		filepath.Join(rootPath, "config", "ambient.yml"),
		filepath.Join(rootPath, paths.ProjectDataDir, "ambient.yaml"),
		filepath.Join(rootPath, paths.ProjectDataDir, "ambient.yml"),
		filepath.Join(rootPath, "config", "zqk.yaml"),
	}
	for _, p := range yamlCandidates {
		if data, err := fileutil.ReadFile(p); err == nil && len(data) > 0 {
			var parsed ambientConfigYAML
			if err := yaml.Unmarshal(data, &parsed); err == nil {
				cfg.IgnoredDirs = append(cfg.IgnoredDirs, parsed.IgnoredDirs...)
				cfg.IgnoredDirs = append(cfg.IgnoredDirs, parsed.Watcher.IgnoredDirs...)
				cfg.IgnoredDirs = append(cfg.IgnoredDirs, parsed.Ambient.IgnoredDirs...)
				cfg.IgnoredDirs = append(cfg.IgnoredDirs, parsed.Ambient.Watcher.IgnoredDirs...)

				cfg.IgnoredPaths = append(cfg.IgnoredPaths, parsed.IgnoredPaths...)
				cfg.IgnoredPaths = append(cfg.IgnoredPaths, parsed.Watcher.IgnoredPaths...)
				cfg.IgnoredPaths = append(cfg.IgnoredPaths, parsed.Ambient.IgnoredPaths...)
				cfg.IgnoredPaths = append(cfg.IgnoredPaths, parsed.Ambient.Watcher.IgnoredPaths...)

				cfg.WatchDirs = append(cfg.WatchDirs, parsed.WatchDirs...)
				cfg.WatchDirs = append(cfg.WatchDirs, parsed.Watcher.WatchDirs...)
				cfg.WatchDirs = append(cfg.WatchDirs, parsed.Ambient.WatchDirs...)
				cfg.WatchDirs = append(cfg.WatchDirs, parsed.Ambient.Watcher.WatchDirs...)
			}
		}
	}

	// 2. Try .ambientignore in rootPath
	ambientIgnorePath := filepath.Join(rootPath, ".ambientignore")
	if data, err := fileutil.ReadFile(ambientIgnorePath); err == nil {
		parseIgnoreFileLines(data, &cfg)
	}

	// 3. Try .gitignore in rootPath to inherit git-ignored directories/build artifacts
	gitIgnorePath := filepath.Join(rootPath, ".gitignore")
	if data, err := fileutil.ReadFile(gitIgnorePath); err == nil {
		parseIgnoreFileLines(data, &cfg)
	}

	// 4. Environment overrides
	if envWatchDirs := zqkenv.AmbientWatchDirs().Get(); envWatchDirs != "" {
		for _, d := range strings.Split(envWatchDirs, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				cfg.WatchDirs = append(cfg.WatchDirs, d)
			}
		}
	}
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
	targets := []string{fw.rootPath}
	if len(fw.config.WatchDirs) > 0 {
		targets = make([]string, 0, len(fw.config.WatchDirs))
		for _, wd := range fw.config.WatchDirs {
			wd = strings.TrimSpace(wd)
			if wd == "" {
				continue
			}
			if filepath.IsAbs(wd) {
				targets = append(targets, wd)
			} else {
				targets = append(targets, filepath.Join(fw.rootPath, wd))
			}
		}
	}

	for _, target := range targets {
		info, err := fileutil.Stat(target)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			_ = fw.watcher.Add(target)
			continue
		}
		// Walk the target directory and add all directories to watcher, ignoring configured ignores
		err = filepath.Walk(target, func(path string, info fileutil.FileInfo, err error) error {
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

// BindProcessYAMLInvalidator subscribes to filesystem events on hub and invalidates/updates the object ID cache for process YAML files.
func BindProcessYAMLInvalidator(hub EventHub, onActivity func()) {
	hub.Subscribe(EventTypeFilesystem, func(c context.Context, event Event) error {
		if onActivity != nil {
			onActivity()
		}
		payloadMap, ok := event.Payload.(map[string]any)
		if !ok {
			return nil
		}
		target, ok := payloadMap[objects.FieldKeyTargetID].(string)
		if !ok {
			return nil
		}
		op, _ := payloadMap[objects.FieldKeyOperation].(string)

		// Only process yaml files in process directories
		if !strings.Contains(target, paths.ProcessDir+"/") || !strings.HasSuffix(target, ".yaml") {
			return nil
		}

		parts := strings.Split(target, string(filepath.Separator))
		for i, part := range parts {
			if part == "process" && i+2 < len(parts) {
				kind := parts[i+1]
				filename := parts[len(parts)-1]
				id := strings.TrimSuffix(filename, ".yaml")

				if op == "REMOVE" {
					objectidcache.InvalidateObjectIDCache(id)
				} else if op == "WRITE" || op == "CREATE" {
					_ = objectidcache.UpdateObjectIDCache(id, kind, target)
				}
				break
			}
		}
		return nil
	})
}
