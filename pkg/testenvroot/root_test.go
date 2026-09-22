package testenvroot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestSetup(t *testing.T) {
	tmp := t.TempDir()
	root, err := Setup(tmp)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if root != tmp {
		t.Fatalf("expected root %q, got %q", tmp, root)
	}

	// Verify test settings file created
	settingsFile := filepath.Join(tmp, paths.ConfigDir, paths.ZqkTestConfigFileName)
	if _, err := os.Stat(settingsFile); err != nil {
		t.Fatalf("test settings file was not created: %v", err)
	}

	// Verify process dirs created
	processDir := filepath.Join(tmp, paths.ProcessDir)
	if _, err := os.Stat(processDir); err != nil {
		t.Fatalf("process dir was not created: %v", err)
	}
}
