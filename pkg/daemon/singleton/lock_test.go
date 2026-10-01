package singleton

import (
	"errors"
	"os"
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

func TestRunGuarded_ExecutesAndReleases(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	executed := false
	err := RunGuarded(tmpDir, "steward", func() error {
		executed = true
		// Verify lock is held inside the closure
		running, _, checkErr := IsDaemonRunning(tmpDir, "steward")
		if checkErr != nil || !running {
			t.Errorf("expected steward daemon to be running inside RunGuarded, running=%v, err=%v", running, checkErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunGuarded returned unexpected error: %v", err)
	}
	if !executed {
		t.Fatal("expected RunGuarded action to execute")
	}

	// Lock should be released after RunGuarded exits
	running, _, checkErr := IsDaemonRunning(tmpDir, "steward")
	if checkErr != nil || running {
		t.Errorf("expected steward daemon lock to be released after RunGuarded, running=%v, err=%v", running, checkErr)
	}
}

func TestGuard_AcquiresAndReleases(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	release, err := Guard(tmpDir, "overseer")
	if err != nil {
		t.Fatalf("Guard failed unexpectedly: %v", err)
	}

	// Verify lock is held
	running, _, checkErr := IsDaemonRunning(tmpDir, "overseer")
	if checkErr != nil || !running {
		t.Errorf("expected overseer daemon to be running while Guard is held, running=%v, err=%v", running, checkErr)
	}

	// Secondary acquisition should fail
	_, err2 := Guard(tmpDir, "overseer")
	if err2 == nil {
		t.Fatal("expected secondary Guard acquisition to fail while held")
	}

	release()

	// Lock should now be released
	running, _, checkErr = IsDaemonRunning(tmpDir, "overseer")
	if checkErr != nil || running {
		t.Errorf("expected overseer daemon lock to be released after release(), running=%v, err=%v", running, checkErr)
	}
}

func TestActiveDaemonPIDs(t *testing.T) {
	tmpDir := t.TempDir()

	pids, err := ActiveDaemonPIDs(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pids) != 0 {
		t.Fatalf("expected 0 active pids, got %d", len(pids))
	}

	release1, err := Guard(tmpDir, "overseer")
	if err != nil {
		t.Fatalf("Guard failed: %v", err)
	}
	defer release1()

	release2, err := Guard(tmpDir, "steward")
	if err != nil {
		t.Fatalf("Guard failed: %v", err)
	}
	defer release2()

	pids, err = ActiveDaemonPIDs(tmpDir)
	if err != nil {
		t.Fatalf("ActiveDaemonPIDs failed: %v", err)
	}
	if len(pids) != 2 {
		t.Fatalf("expected 2 active daemons, got %d", len(pids))
	}

	self := os.Getpid()
	if pids["overseer"] != self || pids["steward"] != self {
		t.Fatalf("expected overseer and steward PID %d, got %v", self, pids)
	}
}
