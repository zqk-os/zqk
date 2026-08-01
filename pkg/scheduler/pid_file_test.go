package scheduler

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestPIDFile_WriteReadRemove(t *testing.T) {
	t.Parallel()
	// Create temporary directory for test
	testRoot := t.TempDir()

	// Test writing PID file
	err := writePIDFile(testRoot)
	if err != nil {
		t.Fatalf("writePIDFile() error = %v", err)
	}

	// Verify PID file exists
	pidFilePath := getPIDFilePath(testRoot)
	if _, err := os.Stat(pidFilePath); os.IsNotExist(err) {
		t.Error("PID file should exist after writePIDFile()")
	}

	// Test reading PID file
	pid, err := readPIDFile(testRoot)
	if err != nil {
		t.Fatalf("readPIDFile() error = %v", err)
	}

	// Verify PID matches current process
	currentPID := os.Getpid()
	if pid != currentPID {
		t.Errorf("readPIDFile() pid = %d, want %d", pid, currentPID)
	}

	// Test removing PID file
	err = removePIDFile(testRoot)
	if err != nil {
		t.Fatalf("removePIDFile() error = %v", err)
	}

	// Verify PID file is removed
	if _, err := os.Stat(pidFilePath); !os.IsNotExist(err) {
		t.Error("PID file should not exist after removePIDFile()")
	}
}

func TestPIDFile_ReadNonExistent(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	_, err := readPIDFile(testRoot)
	if err == nil {
		t.Error("readPIDFile() should return error for non-existent file")
	}
}

func TestIsPIDFileNotExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "file not found", err: errfmt.Errorf("PID file does not exist"), want: true},
		{name: "timeout", err: errfmt.Errorf("timed out reading /tmp/x after 2s"), want: false},
		{name: "wrapped not exist", err: errfmt.Errorf("something: PID file does not exist"), want: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isPIDFileNotExist(tc.err)
			if got != tc.want {
				t.Errorf("isPIDFileNotExist(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsSchedulerRunning_NoRetryOnFileNotExist(t *testing.T) {
	t.Parallel()
	// With no PID file, IsSchedulerRunning should return false without hanging on retry
	testRoot := t.TempDir()
	running, _, err := IsSchedulerRunning(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerRunning() unexpected error: %v", err)
	}
	if running {
		t.Error("IsSchedulerRunning() should return false when no PID file exists")
	}
}

func TestIsSchedulerRunning_NotRunning(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	running, pid, err := IsSchedulerRunning(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerRunning() error = %v", err)
	}

	if running {
		t.Error("IsSchedulerRunning() should return false when PID file doesn't exist")
	}

	if pid != 0 {
		t.Errorf("IsSchedulerRunning() pid = %d, want 0 when not running", pid)
	}
}

func TestIsSchedulerRunning_Running(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	// Write PID file with current process PID
	err := writePIDFile(testRoot)
	if err != nil {
		t.Fatalf("writePIDFile() error = %v", err)
	}

	// Check if scheduler is running
	running, pid, err := IsSchedulerRunning(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerRunning() error = %v", err)
	}

	if !running {
		t.Error("IsSchedulerRunning() should return true when PID file exists and process is running")
	}

	currentPID := os.Getpid()
	if pid != currentPID {
		t.Errorf("IsSchedulerRunning() pid = %d, want %d", pid, currentPID)
	}

	// Cleanup
	_ = removePIDFile(testRoot)
}

func TestIsSchedulerRunning_StalePIDFile(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	// Write PID file with a non-existent PID (use a very high number that won't exist)
	pidFilePath := getPIDFilePath(testRoot)
	schedulerDir := filepath.Dir(pidFilePath)
	if err := os.MkdirAll(schedulerDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create scheduler directory: %v", err)
	}

	// Use a PID that definitely doesn't exist (max int32 is 2147483647, use something reasonable)
	if err := os.WriteFile(pidFilePath, []byte("999999999"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Check if scheduler is running - should detect stale PID file
	running, pid, err := IsSchedulerRunning(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerRunning() error = %v", err)
	}

	if running {
		t.Error("IsSchedulerRunning() should return false for stale PID file")
	}

	if pid != 0 {
		t.Errorf("IsSchedulerRunning() pid = %d, want 0 for stale PID", pid)
	}

	// Verify stale PID file was cleaned up
	if _, err := os.Stat(pidFilePath); !os.IsNotExist(err) {
		t.Error("Stale PID file should be removed by IsSchedulerRunning()")
	}
}

func TestStopSchedulerByPID_NotRunning(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	err := StopSchedulerByPID(testRoot)
	if err == nil {
		t.Error("StopSchedulerByPID() should return error when scheduler is not running")
	}
}

func TestSignalSchedulerByPID_NotRunning(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()
	_, err := SignalSchedulerByPID(testRoot, 0)
	if err == nil {
		t.Error("SignalSchedulerByPID() should return error when scheduler is not running")
	}
}

func TestWaitForSchedulerDaemonExit_RemovesPIDWhenProcessGone(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()
	pidFilePath := getPIDFilePath(testRoot)
	schedulerDir := filepath.Dir(pidFilePath)
	if err := os.MkdirAll(schedulerDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFilePath, []byte("999999998"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := WaitForSchedulerDaemonExit(testRoot, 999999998, time.Second); err != nil {
		t.Fatalf("WaitForSchedulerDaemonExit: %v", err)
	}
	if _, err := os.Stat(pidFilePath); !os.IsNotExist(err) {
		t.Fatal("PID file should be removed after process is gone")
	}
}

func TestWaitForSchedulerDaemonExit_TimeoutKeepsPIDFileWhileAlive(t *testing.T) {
	testRoot := t.TempDir()
	pidFilePath := getPIDFilePath(testRoot)
	schedulerDir := filepath.Dir(pidFilePath)
	if err := os.MkdirAll(schedulerDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	me := os.Getpid()
	if err := os.WriteFile(pidFilePath, []byte(strconv.Itoa(me)), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	err := WaitForSchedulerDaemonExit(testRoot, me, 80*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error while this process is still alive")
	}
	if _, statErr := os.Stat(pidFilePath); statErr != nil {
		t.Fatalf("PID file should still exist after timeout: %v", statErr)
	}
	_ = removePIDFile(testRoot)
}

func TestForceKillSchedulerByPID_NotRunning(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()
	err := ForceKillSchedulerByPID(testRoot)
	if err == nil {
		t.Error("ForceKillSchedulerByPID() should return error when scheduler is not running")
	}
}

func TestGetPIDFilePath(t *testing.T) {
	t.Parallel()
	testRoot := "/tmp/test-project"
	expected := filepath.Join(testRoot, paths.ProjectDataDir, "scheduler", DefaultPIDFileName)

	actual := getPIDFilePath(testRoot)
	if actual != expected {
		t.Errorf("getPIDFilePath() = %s, want %s", actual, expected)
	}
}

func TestIsZombieProcess_InvalidPID(t *testing.T) {
	t.Parallel()
	if isZombieProcess(999999999) {
		t.Error("isZombieProcess should be false for invalid PID")
	}
}

// TestFindSchedulerProcessesByCommand_Completes verifies the go-ps-based process scan
// completes without error and returns a valid map. When run under "go test" the test
// binary is not named "zqk", so the map is typically empty; the important part is
// that the OS process listing and timeout path work reliably.
func TestFindSchedulerProcessesByCommand_Completes(t *testing.T) {
	t.Parallel()

	m, err := findSchedulerProcessesByCommand()
	if err != nil {
		t.Fatalf("findSchedulerProcessesByCommand() error = %v", err)
	}
	if m == nil {
		t.Error("findSchedulerProcessesByCommand() returned nil map")
	}
	// When no zqk process exists (e.g. under go test), map is empty; that's expected.
	_ = m
}
