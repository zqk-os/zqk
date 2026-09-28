package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	testJobIDSample = "JOB-SAMPLE-001"
	testDeadPID     = 9999999
)

func TestIsProcessAlive(t *testing.T) {
	currentPID := os.Getpid()
	if !IsProcessAlive(currentPID) {
		t.Fatalf("expected current PID %d to be alive", currentPID)
	}

	if IsProcessAlive(0) {
		t.Fatalf("PID 0 should not be reported alive")
	}

	if IsProcessAlive(-1) {
		t.Fatalf("negative PID should not be reported alive")
	}

	if IsProcessAlive(testDeadPID) {
		t.Fatalf("unlikely PID %d was reported alive", testDeadPID)
	}
}

func TestLockMetadata_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	meta := &LockMetadata{
		PID:       os.Getpid(),
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		JobID:     testJobIDSample,
	}

	if err := WriteLockMetadata(lockPath, meta); err != nil {
		t.Fatalf("WriteLockMetadata failed: %v", err)
	}

	readMeta, err := ReadLockMetadata(lockPath)
	if err != nil {
		t.Fatalf("ReadLockMetadata failed: %v", err)
	}

	if readMeta.PID != meta.PID {
		t.Fatalf("expected PID %d, got %d", meta.PID, readMeta.PID)
	}

	if readMeta.JobID != meta.JobID {
		t.Fatalf("expected JobID %s, got %s", meta.JobID, readMeta.JobID)
	}
}

func TestReconcileStaleLock_DeadPID(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "dead_process.lock")

	meta := &LockMetadata{
		PID:       testDeadPID,
		CreatedAt: time.Now().Add(-10 * time.Second),
		JobID:     testJobIDSample,
	}
	if err := WriteLockMetadata(lockPath, meta); err != nil {
		t.Fatalf("WriteLockMetadata failed: %v", err)
	}

	// Lock with dead PID should be reconciled even with large TTL
	cleaned, err := ReconcileStaleLock(lockPath, 1*time.Hour)
	if err != nil {
		t.Fatalf("ReconcileStaleLock failed: %v", err)
	}
	if !cleaned {
		t.Fatalf("expected dead process lock to be cleaned")
	}

	if _, statErr := fileutil.Stat(lockPath); !fileutil.IsNotExist(statErr) {
		t.Fatalf("expected lock file to be removed from disk")
	}
}

func TestReconcileStaleLock_AgeExpiration(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "aged.lock")

	if err := fileutil.WriteFile(lockPath, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	// Set file mod time to 10 minutes ago
	past := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(lockPath, past, past)

	cleaned, err := ReconcileStaleLock(lockPath, 1*time.Minute)
	if err != nil {
		t.Fatalf("ReconcileStaleLock failed: %v", err)
	}
	if !cleaned {
		t.Fatalf("expected aged lock to be cleaned")
	}
}

func TestAcquireSelfHealingLock_RecoversStale(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "recoverable.lock")

	// Seed with dead PID lock
	meta := &LockMetadata{
		PID:       testDeadPID,
		CreatedAt: time.Now().Add(-5 * time.Minute),
	}
	_ = WriteLockMetadata(lockPath, meta)

	// AcquireSelfHealingLock should recover and acquire
	fl, err := AcquireSelfHealingLock(lockPath, 2*time.Second, 1*time.Minute)
	if err != nil {
		t.Fatalf("AcquireSelfHealingLock failed: %v", err)
	}
	defer func() {
		_ = fl.Unlock()
		_ = fl.Close()
	}()

	// Verify new lock metadata has current PID
	newMeta, err := ReadLockMetadata(lockPath)
	if err != nil {
		t.Fatalf("ReadLockMetadata failed: %v", err)
	}
	if newMeta.PID != os.Getpid() {
		t.Fatalf("expected new PID to match current process, got %d", newMeta.PID)
	}
}

func TestCleanStaleLocksWithPID_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create a stale dead PID lock
	stalePath := filepath.Join(tmpDir, "stale.lock")
	_ = WriteLockMetadata(stalePath, &LockMetadata{
		PID: testDeadPID,
	})

	// 2. Create a live fresh lock held by current process
	livePath := filepath.Join(tmpDir, "live.lock")
	fl, err := AcquireSelfHealingLock(livePath, 1*time.Second, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to acquire live lock: %v", err)
	}
	defer func() {
		_ = fl.Unlock()
		_ = fl.Close()
	}()

	cleaned, err := CleanStaleLocksWithPID(tmpDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("CleanStaleLocksWithPID failed: %v", err)
	}

	if cleaned != 1 {
		t.Fatalf("expected 1 lock cleaned, got %d", cleaned)
	}

	// Verify live lock still exists
	if _, statErr := fileutil.Stat(livePath); fileutil.IsNotExist(statErr) {
		t.Fatalf("live lock should NOT have been removed")
	}
}
