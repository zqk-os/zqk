package cleanup

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	testCleanupConfigFilename = "cleanup.yaml"
	testMissingConfigFilename = "nonexistent.yaml"
	testEmptyValue            = ""
	testFilePerm              = 0o600
	testDirPerm               = 0o755
	testDeleteFilesType       = "delete_files"
	testCLIType               = "cli"
	testEchoCommand           = "echo"
	testLogsGlobPath          = ".zqk/logs/**/*.log"
)

func TestLoadFromPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, testCleanupConfigFilename)
	content := []byte(`working_directory: "."
steps:
  - type: delete_files
    path: ".zqk/logs/**/*.log"
    older_than: 7d
  - type: cli
    command: "echo"
    args: ["hello"]
`)
	if err := fileutil.WriteFile(cfgPath, content, testFilePerm); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromPath(dir, testCleanupConfigFilename)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if cfg.WorkingDirectory != "." {
		t.Errorf("working_directory: got %q", cfg.WorkingDirectory)
	}
	if len(cfg.Steps) != 2 {
		t.Fatalf("steps: got %d, want 2", len(cfg.Steps))
	}
	if cfg.Steps[0].Type != testDeleteFilesType {
		t.Errorf("step 0 type: got %q", cfg.Steps[0].Type)
	}
	if p := cfg.Steps[0].Params[objects.FieldKeyPath]; p != testLogsGlobPath {
		t.Errorf("step 0 path: got %v", p)
	}
	if cfg.Steps[1].Type != testCLIType {
		t.Errorf("step 1 type: got %q", cfg.Steps[1].Type)
	}
	if cmd := cfg.Steps[1].Params[objects.FieldKeyCommand]; cmd != testEchoCommand {
		t.Errorf("step 1 command: got %v", cmd)
	}
}

func TestLoadFromPath_defaultPath(t *testing.T) {
	dir := t.TempDir()
	defaultFull := filepath.Join(dir, DefaultConfigPath)
	if err := fileutil.MkdirAll(filepath.Dir(defaultFull), testDirPerm); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(defaultFull, []byte("steps: []"), testFilePerm); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromPath(dir, testEmptyValue)
	if err != nil {
		t.Fatalf("LoadFromPath(empty path): %v", err)
	}
	if cfg == nil || len(cfg.Steps) != 0 {
		t.Errorf("expected empty steps, got %+v", cfg)
	}
}

func TestLoadFromPath_missing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadFromPath(dir, testMissingConfigFilename)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !fileutil.IsNotExist(err) && err.Error() == testEmptyValue {
		t.Errorf("expected non-empty error message: %v", err)
	}
}
