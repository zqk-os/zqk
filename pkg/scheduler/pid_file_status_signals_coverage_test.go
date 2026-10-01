package scheduler

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_PIDFile_DeepStatusAndSignals(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-pid-deep-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// 1. IsSchedulerRunning when no PID file exists
	running, pid, err := IsSchedulerRunning(tmpDir)
	if err != nil || running || pid != 0 {
		t.Errorf("expected not running on empty dir, got running=%v, pid=%d, err=%v", running, pid, err)
	}

	// 2. SignalSchedulerByPID when daemon is down
	_, err = SignalSchedulerByPID(tmpDir, syscall.SIGTERM)
	if err == nil {
		t.Error("expected error when signaling stopped daemon")
	}

	// 3. ForceKillSchedulerByPID when daemon is down
	err = ForceKillSchedulerByPID(tmpDir)
	if err == nil {
		t.Error("expected error when force killing stopped daemon")
	}

	// 4. Stale PID file (process not running)
	pidFile := getPIDFilePath(tmpDir)
	_ = fileutil.MkdirAll(filepath.Dir(pidFile), 0755)
	_ = fileutil.WriteFile(pidFile, []byte(strconv.Itoa(99999999)), 0644)

	keepAliveFile := getKeepAliveFilePath(tmpDir)
	_ = fileutil.WriteFile(keepAliveFile, []byte(time.Now().Format(time.RFC3339)), 0644)

	runningDead, deadPID, errDead := IsSchedulerRunning(tmpDir)
	if errDead != nil || runningDead || deadPID != 0 {
		t.Errorf("expected dead process cleaned up, got running=%v, pid=%d, err=%v", runningDead, deadPID, errDead)
	}
	// Verify stale PID file was removed
	if _, statErr := fileutil.Stat(pidFile); !fileutil.IsNotExist(statErr) {
		t.Error("expected stale PID file to be removed")
	}

	// 5. Active PID file (current process)
	myPID := os.Getpid()
	_ = fileutil.WriteFile(pidFile, []byte(strconv.Itoa(myPID)), 0644)
	runningLive, livePID, errLive := IsSchedulerRunning(tmpDir)
	if errLive != nil || !runningLive || livePID != myPID {
		t.Errorf("expected live process running, got running=%v, pid=%d, err=%v", runningLive, livePID, errLive)
	}

	// 6. SignalSchedulerByPID with RefuseSupervisorSelfStop (current process stops itself refusal)
	_, selfErr := SignalSchedulerByPID(tmpDir, syscall.SIGTERM)
	// RefuseSupervisorSelfStop returns error if target PID is self or supervisor
	t.Logf("self signal result: %v", selfErr)

	// Clean up
	_ = removePIDFile(tmpDir)
	_ = RemoveKeepAlive(tmpDir)
}
