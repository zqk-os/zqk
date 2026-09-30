package singleton

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAcquireDaemonLock_SingleInstancePerProjectRoot(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Initialize valid root
	if err := fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	// First instance acquires lock successfully
	lock1, err := AcquireDaemonLock(tmpDir, "ambient")
	if err != nil {
		t.Fatalf("expected first acquire to succeed, got: %v", err)
	}
	defer lock1.Release()

	// Verify IsDaemonRunning reports true
	running, pid, err := IsDaemonRunning(tmpDir, "ambient")
	if err != nil {
		t.Fatalf("unexpected error checking running status: %v", err)
	}
	if !running {
		t.Fatalf("expected ambient daemon to be reported as running")
	}
	if pid != lock1.PID {
		t.Fatalf("expected reported PID %d, got %d", lock1.PID, pid)
	}

	// Second instance for same daemon name MUST fail
	lock2, err := AcquireDaemonLock(tmpDir, "ambient")
	if err == nil {
		if lock2 != nil {
			lock2.Release()
		}
		t.Fatalf("expected duplicate daemon acquire to fail, but succeeded")
	}

	var alreadyRunning *ErrDaemonAlreadyRunning
	if !errors.As(err, &alreadyRunning) {
		t.Fatalf("expected ErrDaemonAlreadyRunning, got: %v", err)
	}
	if alreadyRunning.DaemonName != "ambient" {
		t.Fatalf("expected daemon name 'ambient', got %q", alreadyRunning.DaemonName)
	}
	if alreadyRunning.PID != lock1.PID {
		t.Fatalf("expected owner PID %d, got %d", lock1.PID, alreadyRunning.PID)
	}

	// Distinct daemon name on same root succeeds
	lockOther, err := AcquireDaemonLock(tmpDir, "privileged-writer")
	if err != nil {
		t.Fatalf("expected distinct daemon to acquire lock, got: %v", err)
	}
	defer lockOther.Release()

	// Release first lock, then acquire again succeeds
	if err := lock1.Release(); err != nil {
		t.Fatalf("failed to release lock1: %v", err)
	}

	lock1Reacquired, err := AcquireDaemonLock(tmpDir, "ambient")
	if err != nil {
		t.Fatalf("expected reacquire after release to succeed, got: %v", err)
	}
	defer lock1Reacquired.Release()
}

func TestAcquireDaemonLock_RejectsNestedProjectRoot(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Top level has .zqk
	if err := fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create top data dir: %v", err)
	}

	// Subfolder also has .zqk (nested violation)
	subDir := filepath.Join(tmpDir, "nested", "subproject")
	if err := fileutil.MkdirAll(filepath.Join(subDir, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create nested data dir: %v", err)
	}

	// Attempting to acquire a daemon lock on the nested subproject root must be rejected
	_, err := AcquireDaemonLock(subDir, "scheduler")
	if err == nil {
		t.Fatalf("expected daemon lock acquire on nested root to fail, but it succeeded")
	}
	if !errors.Is(err, paths.ErrNestedProjectRoot) {
		t.Fatalf("expected error wrapping paths.ErrNestedProjectRoot, got: %v", err)
	}
}
