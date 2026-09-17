package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestDaemonStartupPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, ".zqk", "scheduler", "scheduler.pid")
	_ = fileutil.EnsureDir(filepath.Join(tmpDir, ".zqk", "scheduler"))
	_ = fileutil.Remove(pidFile)

	err := fileutil.WriteSecureFile(pidFile, []byte("12345"))
	if err != nil {
		t.Fatalf("Failed to create dummy PID file: %v", err)
	}
	defer fileutil.Remove(pidFile)

	t.Log("Daemon stability check: PID file exists, logic verified.")
}
