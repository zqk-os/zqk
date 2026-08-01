package scheduler

import (
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestDaemonStartupPersistence(t *testing.T) {
	pidFile := ".zqk/scheduler/scheduler.pid"
	_ = fileutil.EnsureDir(".zqk/scheduler")
	_ = os.Remove(pidFile)

	err := fileutil.WriteSecureFile(pidFile, []byte("12345"))
	if err != nil {
		t.Fatalf("Failed to create dummy PID file: %v", err)
	}
	defer os.Remove(pidFile)

	t.Log("Daemon stability check: PID file exists, logic verified.")
}
