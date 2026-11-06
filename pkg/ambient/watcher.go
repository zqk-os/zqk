package ambient

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
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
		".agent",
		".agents",
		".cursor",
		".ide",
		"bin",
		"dist",
		".venv",
		"env",
		"__pycache__",
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
		"/.agent/",
		"/.agent",
		"/.agents/",
		"/.agents",
		"/.cursor/",
		"/.cursor",
		"/.ide/",
		"/.ide",
		"/bin/",
		"/bin",
		"/dist/",
		"/dist",
		"/.venv/",
		"/.venv",
		"/env/",
		"/env",
		"/__pycache__/",
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
	stopOnce     sync.Once
	stopErr      error
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
	if fw == nil {
		return fmt.Errorf("cannot start nil FSWatcher")
	}
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
			if addErr := fw.watcher.Add(target); addErr != nil {
				// non-fatal watcher add error
			}
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
							if addErr := fw.watcher.Add(event.Name); addErr != nil {
								// non-fatal watcher add error
							}
						}
						continue
					}

					// Publish event
					if pubErr := fw.eventHub.Publish(context.Background(), Event{
						Type: EventTypeFilesystem,
						Payload: map[string]any{
							objects.FieldKeyTargetID:  event.Name,
							objects.FieldKeyOperation: event.Op.String(),
						},
						Timestamp: time.Now(),
					}); pubErr != nil {
						// non-fatal publish error
					}
				}
			case <-fw.watcher.Errors:
				// just ignore for now
			}
		}
	})

	return nil
}

// Stop halts the watcher idempotently without double-close panics.
func (fw *FSWatcher) Stop() error {
	if fw == nil {
		return nil
	}
	fw.stopOnce.Do(func() {
		close(fw.done)
		fw.stopErr = fw.watcher.Close()
	})
	return fw.stopErr
}

// BindProcessYAMLInvalidator subscribes to filesystem events on hub and invalidates/updates the object ID cache for process YAML files.
func BindProcessYAMLInvalidator(hub EventHub, onActivity func()) {
	if hub == nil {
		return
	}
	hub.Subscribe(EventTypeFilesystem, func(c context.Context, event Event) error {
		defer func() {
			if r := recover(); r != nil {
				// contained rogue onActivity or invalidator panic
			}
		}()
		if onActivity != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						// contained activity panic
					}
				}()
				onActivity()
			}()
		}
		payloadMap, ok := event.Payload.(map[string]any)
		if !ok {
			return nil
		}
		target, ok := payloadMap[objects.FieldKeyTargetID].(string)
		if !ok {
			return nil
		}
		op, ok := payloadMap[objects.FieldKeyOperation].(string)
		if !ok {
			op = ""
		}

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
					if updateErr := objectidcache.UpdateObjectIDCache(id, kind, target); updateErr != nil {
						// Non-fatal cache update failure
					}
				}
				break
			}
		}
		return nil
	})
}

// Default timing constants for ambient watcher daemon execution.
const (
	// DefaultTelemetryInterval is the default periodic interval for ambient telemetry report persistence.
	DefaultTelemetryInterval = 5 * time.Second
)

// Sentinel errors for ambient watcher daemon lifecycle, configuration, and bounds.
var (
	// ErrInvalidProjectRoot is returned when the project root path is empty or invalid.
	ErrInvalidProjectRoot = errfmt.Errorf("project root is required")
	// ErrNilContext is returned when a nil context is passed to RunWatcherDaemonWithConfig.
	ErrNilContext = errfmt.Errorf("context cannot be nil")
)

// WatcherDaemonConfig defines configuration and lifecycle parameters for RunWatcherDaemonWithConfig.
type WatcherDaemonConfig struct {
	// ProjectRoot specifies the workspace root path monitored by the watcher daemon.
	ProjectRoot string
	// OnActivity is an optional callback invoked when relevant filesystem modifications occur.
	OnActivity func()
	// EngineConfig defines configuration parameters for the embedded AmbientEngine.
	// If nil, defaults to DefaultAmbientEngineConfig().
	EngineConfig *AmbientEngineConfig
	// WatcherConfig optionally defines filesystem watcher filtering rules and directories.
	// If nil, defaults to LoadWatcherConfig(ProjectRoot).
	WatcherConfig *WatcherConfig
	// TelemetryInterval defines the periodic interval for telemetry persistence.
	// If <= 0, defaults to DefaultTelemetryInterval (5s).
	TelemetryInterval time.Duration
	// EnableEngine controls whether the embedded AmbientEngine is instantiated and started.
	// Defaults to true when initialized via DefaultWatcherDaemonConfig.
	EnableEngine bool
	// Engine optionally supplies an external or pre-instantiated AmbientEngine.
	Engine *AmbientEngine
	// OnEngineStarted is an optional lifecycle callback invoked immediately after the engine starts.
	OnEngineStarted func(engine *AmbientEngine)
	// StatePath optionally defines a custom file path for persisted telemetry reports.
	StatePath string
}

// DefaultWatcherDaemonConfig returns the production default configuration for the ambient watcher daemon.
func DefaultWatcherDaemonConfig(projectRoot string) WatcherDaemonConfig {
	engineCfg := DefaultAmbientEngineConfig()
	return WatcherDaemonConfig{
		ProjectRoot:       projectRoot,
		OnActivity:        nil,
		EngineConfig:      &engineCfg,
		TelemetryInterval: DefaultTelemetryInterval,
		EnableEngine:      true,
	}
}

func validateDaemonRoot(projectRoot string) error {
	if strings.TrimSpace(projectRoot) == "" {
		return ErrInvalidProjectRoot
	}
	info, err := fileutil.Stat(projectRoot)
	if err != nil {
		return fmt.Errorf("invalid project root %q: %w", projectRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("invalid project root %q is not a directory", projectRoot)
	}
	return nil
}

func setupDaemonEngine(ctx context.Context, cfg WatcherDaemonConfig) (*AmbientEngine, bool, error) {
	if cfg.Engine != nil {
		return resolveAmbientEngine(ctx, cfg.Engine)
	}
	if !cfg.EnableEngine {
		return nil, false, nil
	}
	if cfg.EngineConfig != nil {
		eng, err := NewAmbientEngine(*cfg.EngineConfig)
		if err != nil {
			return nil, false, fmt.Errorf("failed to create ambient engine: %w", err)
		}
		if err := eng.Start(ctx); err != nil {
			return nil, false, fmt.Errorf("failed to start ambient engine: %w", err)
		}
		return eng, true, nil
	}
	return resolveAmbientEngine(ctx, nil)
}

func setupFSWatcher(ctx context.Context, projectRoot string, hub EventHub, watcherCfg *WatcherConfig) (*FSWatcher, error) {
	var watcher *FSWatcher
	var err error
	if watcherCfg != nil {
		watcher, err = NewFSWatcherWithConfig(projectRoot, hub, *watcherCfg)
	} else {
		watcher, err = NewFSWatcher(projectRoot, hub)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create fswatcher: %w", err)
	}
	if err := watcher.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start fswatcher: %w", err)
	}
	return watcher, nil
}

// RunWatcherDaemonWithConfig orchestrates the ambient watcher pipeline (event hub, artifact writer,
// coach heuristics, ingest service, fswatcher, and process YAML invalidator) alongside the hosted
// AmbientEngine runtime and periodic telemetry persistence until ctx is cancelled.
func RunWatcherDaemonWithConfig(ctx context.Context, cfg WatcherDaemonConfig) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := validateDaemonRoot(cfg.ProjectRoot); err != nil {
		return err
	}

	interval := cfg.TelemetryInterval
	if interval <= 0 {
		interval = DefaultTelemetryInterval
	}

	engine, created, err := setupDaemonEngine(ctx, cfg)
	if err != nil {
		return err
	}
	if engine != nil {
		defer func() {
			if created {
				if stopErr := engine.Stop(); stopErr != nil && !errors.Is(stopErr, ErrEngineStopped) {
					// graceful engine stop cleanup
				}
			}
		}()
		if cfg.OnEngineStarted != nil {
			cfg.OnEngineStarted(engine)
		}
	}

	hub := NewEventHub()
	ingestSvc, err := initWatcherAuxiliaries(hub, cfg.ProjectRoot, cfg.OnActivity)
	if err != nil {
		return err
	}
	defer func() {
		if ingestSvc != nil {
			ingestSvc.Stop()
		}
		hub.WaitForAudits()
	}()

	watcher, err := setupFSWatcher(ctx, cfg.ProjectRoot, hub, cfg.WatcherConfig)
	if err != nil {
		return err
	}
	defer func() {
		if stopErr := watcher.Stop(); stopErr != nil {
			// graceful watcher stop cleanup
		}
	}()

	var waitPersister func()
	if engine != nil {
		if strings.TrimSpace(cfg.StatePath) != "" {
			engine.SetStatePath(cfg.StatePath)
		}
		waitPersister = startTelemetryPersister(ctx, cfg.ProjectRoot, engine, interval)
	}

	<-ctx.Done()
	if waitPersister != nil {
		waitPersister()
	}
	if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(ctxErr, context.Canceled) && !errors.Is(ctxErr, context.DeadlineExceeded) {
		return ctxErr
	}
	return nil
}

// RunWatcherDaemonWithEngine orchestrates the ambient watcher pipeline alongside a provided AmbientEngine instance.
func RunWatcherDaemonWithEngine(ctx context.Context, projectRoot string, engine *AmbientEngine, onActivity func()) error {
	cfg := DefaultWatcherDaemonConfig(projectRoot)
	cfg.OnActivity = onActivity
	cfg.Engine = engine
	if engine == nil {
		cfg.EnableEngine = true
	}
	return RunWatcherDaemonWithConfig(ctx, cfg)
}

// RunWatcherDaemonWithEngineAndConfig runs the watcher daemon with explicit WatcherConfig and AmbientEngine.
func RunWatcherDaemonWithEngineAndConfig(ctx context.Context, projectRoot string, watcherCfg WatcherConfig, engine *AmbientEngine, onActivity func()) error {
	cfg := DefaultWatcherDaemonConfig(projectRoot)
	cfg.WatcherConfig = &watcherCfg
	cfg.Engine = engine
	cfg.OnActivity = onActivity
	if engine == nil {
		cfg.EnableEngine = true
	}
	return RunWatcherDaemonWithConfig(ctx, cfg)
}

func initWatcherAuxiliaries(hub EventHub, projectRoot string, onActivity func()) (*AmbientIngestService, error) {
	if writer := NewArtifactWriter(hub); writer == nil {
		return nil, fmt.Errorf("failed to create artifact writer")
	}
	if coach := NewCoachHeuristicsWithRoot(hub, projectRoot); coach == nil {
		return nil, fmt.Errorf("failed to create coach heuristics")
	}
	secCtx := pkgctx.NewSecurityContext(pkgctx.SystemAccountID, []string{"system"}, []string{"*"})
	ingestService := NewAmbientIngestService(projectRoot, secCtx)
	ingestService.BindToHub(hub)
	BindProcessYAMLInvalidator(hub, onActivity)
	return ingestService, nil
}

func resolveAmbientEngine(ctx context.Context, engine *AmbientEngine) (*AmbientEngine, bool, error) {
	if engine != nil {
		if !engine.IsRunning() {
			if err := engine.Start(ctx); err != nil {
				return nil, false, fmt.Errorf("failed to start ambient engine: %w", err)
			}
			return engine, true, nil
		}
		return engine, false, nil
	}

	cfg := DefaultAmbientEngineConfig()
	cfg.TickInterval = 1 * time.Second
	cfg.GracefulStopTimeout = 3 * time.Second
	created, err := NewAmbientEngine(cfg)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create ambient engine: %w", err)
	}
	if err := created.Start(ctx); err != nil {
		return nil, false, fmt.Errorf("failed to start ambient engine: %w", err)
	}
	return created, true, nil
}

func startTelemetryPersister(ctx context.Context, projectRoot string, engine *AmbientEngine, intervals ...time.Duration) func() {
	if strings.TrimSpace(projectRoot) == "" || engine == nil {
		return func() {}
	}
	interval := DefaultTelemetryInterval
	if len(intervals) > 0 && intervals[0] > 0 {
		interval = intervals[0]
	}

	targetPath := engine.StatePath()
	if targetPath == "" {
		targetPath = filepath.Join(projectRoot, paths.AmbientTelemetryLatestRelFile)
		engine.SetStatePath(targetPath)
	}
	legacyPath := filepath.Join(paths.StateDirPath(projectRoot), "ambient", "telemetry.json")

	if ensureErr := fileutil.EnsureDir(filepath.Dir(targetPath)); ensureErr != nil {
		// non-fatal target directory creation error
	}
	if ensureErr := fileutil.EnsureDir(filepath.Dir(legacyPath)); ensureErr != nil {
		// non-fatal legacy directory creation error
	}

	persistSnapshots := func(running bool) {
		runningReport := engine.TelemetryReport()
		latestReport := runningReport.Clone()
		if latestReport != nil {
			latestReport.Status = TelemetryStatusRunning
		}
		if saveErr := SaveTelemetryReport(targetPath, latestReport); saveErr != nil {
			// non-fatal save error
		}

		legacyReport := runningReport.Clone()
		if !running && legacyReport != nil {
			legacyReport.Status = TelemetryStatusStopped
		}
		if saveErr := SaveTelemetryReport(legacyPath, legacyReport); saveErr != nil {
			// non-fatal save error
		}
	}

	persistSnapshots(true)

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("ambient_telemetry_persister", "periodically persist ambient telemetry report to disk").
		StartSimple(func() {
			defer close(done)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					persistSnapshots(false)
					return
				case <-ticker.C:
					persistSnapshots(true)
				}
			}
		})

	return func() {
		<-done
	}
}

// RunWatcherDaemon orchestrates the standard ambient watcher pipeline and hosted AmbientEngine until ctx is cancelled.
func RunWatcherDaemon(ctx context.Context, projectRoot string, onActivity func()) error {
	cfg := DefaultWatcherDaemonConfig(projectRoot)
	cfg.OnActivity = onActivity
	return RunWatcherDaemonWithConfig(ctx, cfg)
}
