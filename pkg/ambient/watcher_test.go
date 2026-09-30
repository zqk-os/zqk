package ambient

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFSWatcher_FiltersNoise(t *testing.T) {
	// Setup
	tmpDir, err := fileutil.MkdirTemp("", "fswatcher-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	// Create noise directory
	gitDir := filepath.Join(tmpDir, ".git")
	_ = fileutil.EnsureDir(gitDir)

	hub := NewEventHub()
	watcher, err := NewFSWatcher(tmpDir, hub)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = watcher.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer func() { _ = watcher.Stop() }()

	// Subscribe to events
	received := make(chan Event, 1)
	hub.Subscribe(EventTypeFilesystem, func(ctx context.Context, event Event) error {
		received <- event
		return nil
	})

	// Action 1: Write to a valid file
	validFile := filepath.Join(tmpDir, "main.go")
	_ = fileutil.WriteSecureFile(validFile, []byte("package main"))

	// Assert: Should receive event
	select {
	case evt := <-received:
		payload := evt.Payload.(map[string]any)
		if payload[objects.FieldKeyTargetID] != validFile {
			t.Errorf("expected event for %s, got %v", validFile, payload[objects.FieldKeyTargetID])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for valid file event")
	}

	// Action 2: Write to ignored directory
	ignoredFile := filepath.Join(gitDir, "HEAD")
	_ = fileutil.WriteSecureFile(ignoredFile, []byte("ref: refs/heads/main"))

	// Assert: Should NOT receive event
	select {
	case evt := <-received:
		payload := evt.Payload.(map[string]any)
		t.Fatalf("received unexpected event for ignored file: %v", payload[objects.FieldKeyTargetID])
	case <-time.After(500 * time.Millisecond):
		// Success: timeout means the event was properly filtered
	}

	// Action 3: Write to internal .zqk directory (must never trigger ambient event feedback loop)
	zqkDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.ProcessSubdir, "audit")
	_ = fileutil.EnsureDir(zqkDir)
	zqkAuditFile := filepath.Join(zqkDir, "event.yaml")
	_ = fileutil.WriteSecureFile(zqkAuditFile, []byte("id: AUD-1"))

	select {
	case evt := <-received:
		payload := evt.Payload.(map[string]any)
		t.Fatalf("received unexpected event for internal .zqk file: %v", payload[objects.FieldKeyTargetID])
	case <-time.After(500 * time.Millisecond):
		// Success: timeout means the .zqk event was properly filtered
	}

	// Action 4: Write to internal .zqk-state directory
	zqkStateDir := filepath.Join(tmpDir, paths.ProjectStateDir)
	_ = fileutil.EnsureDir(zqkStateDir)
	zqkStateFile := filepath.Join(zqkStateDir, "system-state.csnap")
	_ = fileutil.WriteSecureFile(zqkStateFile, []byte("state-blob"))

	select {
	case evt := <-received:
		payload := evt.Payload.(map[string]any)
		t.Fatalf("received unexpected event for internal .zqk-state file: %v", payload[objects.FieldKeyTargetID])
	case <-time.After(500 * time.Millisecond):
		// Success: timeout means the .zqk-state event was properly filtered
	}
}

func TestLoadWatcherConfig_YAMLAndIgnoreFileAndEnv(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "fswatcher-cfg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	// 1. Defaults only
	cfgDef := LoadWatcherConfig(tmpDir)
	if len(cfgDef.IgnoredDirs) == 0 || len(cfgDef.IgnoredPaths) == 0 {
		t.Errorf("expected non-empty defaults, got %+v", cfgDef)
	}

	// 2. YAML file in .zqk/ambient.yaml
	zqkDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	_ = fileutil.EnsureDir(zqkDir)
	yamlContent := `watcher:
  ignored_dirs:
    - custom_dir_a
    - custom_dir_b
  ignored_paths:
    - /custom_path_x/
`
	_ = fileutil.WriteSecureFile(filepath.Join(zqkDir, "ambient.yaml"), []byte(yamlContent))

	// 3. .ambientignore in root
	ignoreContent := `# Comment
custom_ignore_dir
/custom_ignore_path/
`
	_ = fileutil.WriteSecureFile(filepath.Join(tmpDir, ".ambientignore"), []byte(ignoreContent))

	// 4. Env vars
	t.Setenv("ZQK_AMBIENT_IGNORE_DIRS", "env_dir_1, env_dir_2")
	t.Setenv("ZQK_AMBIENT_IGNORE_PATHS", "/env_path_1/, /env_path_2/")

	cfgLoaded := LoadWatcherConfig(tmpDir)

	hasDir := func(dirs []string, target string) bool {
		for _, d := range dirs {
			if d == target {
				return true
			}
		}
		return false
	}

	for _, expectedDir := range []string{"custom_dir_a", "custom_dir_b", "custom_ignore_dir", "env_dir_1", "env_dir_2"} {
		if !hasDir(cfgLoaded.IgnoredDirs, expectedDir) {
			t.Errorf("missing expected ignored dir: %s in %+v", expectedDir, cfgLoaded.IgnoredDirs)
		}
	}

	hasPath := func(paths []string, target string) bool {
		for _, p := range paths {
			if p == target {
				return true
			}
		}
		return false
	}

	for _, expectedPath := range []string{"/custom_path_x/", "/custom_ignore_path/", "/env_path_1/", "/env_path_2/"} {
		if !hasPath(cfgLoaded.IgnoredPaths, expectedPath) {
			t.Errorf("missing expected ignored path: %s in %+v", expectedPath, cfgLoaded.IgnoredPaths)
		}
	}
}

func TestFSWatcher_CustomConfigFiltering(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "fswatcher-custom-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	hub := NewEventHub()
	customCfg := DefaultWatcherConfig()
	customCfg.IgnoredDirs = append(customCfg.IgnoredDirs, "my_custom_folder")
	customCfg.IgnoredPaths = append(customCfg.IgnoredPaths, "/my_custom_folder/")

	watcher, err := NewFSWatcherWithConfig(tmpDir, hub, customCfg)
	if err != nil {
		t.Fatalf("failed to create custom watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start custom watcher: %v", err)
	}
	defer func() { _ = watcher.Stop() }()

	if !watcher.isIgnoredDirName("my_custom_folder") {
		t.Errorf("expected my_custom_folder to be ignored")
	}
	if !watcher.isIgnoredFSPath("/project/my_custom_folder/file.go") {
		t.Errorf("expected /project/my_custom_folder/file.go to be ignored")
	}
	if watcher.isIgnoredDirName("allowed_src") {
		t.Errorf("allowed_src should not be ignored")
	}
}

func TestLoadWatcherConfig_ConfigAmbientYAMLAndGitIgnoreAndWatchDirs(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "fswatcher-cfg2-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	// 1. config/ambient.yaml
	configDir := filepath.Join(tmpDir, "config")
	_ = fileutil.EnsureDir(configDir)
	yamlContent := `watcher:
  watch_dirs:
    - src
    - internal
  ignored_dirs:
    - docs
    - extra_noise
  ignored_paths:
    - /docs/
`
	_ = fileutil.WriteSecureFile(filepath.Join(configDir, "ambient.yaml"), []byte(yamlContent))

	// 2. .gitignore
	gitIgnoreContent := `# Gitignore
build_output
/coverage_report/
`
	_ = fileutil.WriteSecureFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitIgnoreContent))

	// 3. Env var for watch dirs
	t.Setenv("ZQK_AMBIENT_WATCH_DIRS", "plugins, components")

	cfg := LoadWatcherConfig(tmpDir)

	hasItem := func(slice []string, target string) bool {
		for _, s := range slice {
			if s == target {
				return true
			}
		}
		return false
	}

	for _, expectedDir := range []string{"docs", "extra_noise", "build_output"} {
		if !hasItem(cfg.IgnoredDirs, expectedDir) {
			t.Errorf("missing expected ignored dir: %s in %+v", expectedDir, cfg.IgnoredDirs)
		}
	}

	for _, expectedPath := range []string{"/docs/", "/coverage_report/"} {
		if !hasItem(cfg.IgnoredPaths, expectedPath) {
			t.Errorf("missing expected ignored path: %s in %+v", expectedPath, cfg.IgnoredPaths)
		}
	}

	for _, expectedWatch := range []string{"src", "internal", "plugins", "components"} {
		if !hasItem(cfg.WatchDirs, expectedWatch) {
			t.Errorf("missing expected watch dir: %s in %+v", expectedWatch, cfg.WatchDirs)
		}
	}
}

