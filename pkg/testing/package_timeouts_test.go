package testing

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestGetMinTimeoutSecondsForPackage_Defaults(t *testing.T) {
	// Empty project root: no config loaded, use built-in defaults
	dir := t.TempDir()

	if got := GetMinTimeoutSecondsForPackage(dir, "cmd/zqk/object"); got != 600 {
		t.Errorf("cmd/zqk/object: got %d, want 600", got)
	}
	if got := GetMinTimeoutSecondsForPackage(dir, "cmd/zqk"); got != 600 {
		t.Errorf("cmd/zqk: got %d, want 600", got)
	}
	if got := GetMinTimeoutSecondsForPackage(dir, "pkg/storage"); got != 600 {
		t.Errorf("pkg/storage: got %d, want 600", got)
	}
	if got := GetMinTimeoutSecondsForPackage(dir, "pkg/other"); got != 0 {
		t.Errorf("pkg/other: got %d, want 0", got)
	}
	// With ./ prefix
	if got := GetMinTimeoutSecondsForPackage(dir, "./cmd/zqk/object"); got != 600 {
		t.Errorf("./cmd/zqk/object: got %d, want 600", got)
	}
}

func TestGetMinTimeoutSecondsForPackage_ConfigOverride(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, paths.ProjectDataDir, paths.ConfigDir)
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, paths.ScanTestsPackageTimeoutsConfigFile)
	// Override: pkg/storage -> 900; add pkg/foo -> 120
	yaml := `packages:
  - pattern: "pkg/storage"
    min_seconds: 900
  - pattern: "pkg/foo"
    min_seconds: 120
`
	if err := fileutil.WriteFile(configPath, []byte(yaml), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if got := GetMinTimeoutSecondsForPackage(dir, "pkg/storage"); got != 900 {
		t.Errorf("pkg/storage with override: got %d, want 900", got)
	}
	if got := GetMinTimeoutSecondsForPackage(dir, "pkg/foo"); got != 120 {
		t.Errorf("pkg/foo from config: got %d, want 120", got)
	}
	// Built-in defaults still apply when no rule matches (cmd/zqk from code defaults)
	if got := GetMinTimeoutSecondsForPackage(dir, "cmd/zqk"); got != 600 {
		t.Errorf("cmd/zqk (default when config present but no rule): got %d, want 600", got)
	}
}

func TestGetMinTimeoutSecondsForPackage_LongestMatch(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, paths.ProjectDataDir, paths.ConfigDir)
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, paths.ScanTestsPackageTimeoutsConfigFile)
	// cmd/zqk -> 300, cmd/zqk/object -> 600; longest match should win for cmd/zqk/object
	yaml := `packages:
  - pattern: "cmd/zqk"
    min_seconds: 300
  - pattern: "cmd/zqk/object"
    min_seconds: 600
`
	if err := fileutil.WriteFile(configPath, []byte(yaml), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if got := GetMinTimeoutSecondsForPackage(dir, "cmd/zqk/object"); got != 600 {
		t.Errorf("cmd/zqk/object (longest match): got %d, want 600", got)
	}
	if got := GetMinTimeoutSecondsForPackage(dir, "cmd/zqk"); got != 300 {
		t.Errorf("cmd/zqk: got %d, want 300", got)
	}
}
