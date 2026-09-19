package resourcehygiene

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

func TestGetProcessFDUsage(t *testing.T) {
	fds, limit, err := GetProcessFDUsage()
	if err != nil {
		t.Skipf("FD inspection unsupported or unavailable on this platform: %v", err)
	}
	if fds < 0 {
		t.Errorf("expected non-negative open FDs, got %d", fds)
	}
	if limit <= 0 {
		t.Logf("limit returned %d", limit)
	}
}

func TestReapOrphanedTempFiles(t *testing.T) {
	tmpDir := t.TempDir()
	zqkCache := filepath.Join(tmpDir, paths.ProjectDataDir, "cache")
	if err := fileutil.MkdirAll(zqkCache, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	oldTmp := filepath.Join(zqkCache, ".tmp-durable-9999")
	freshTmp := filepath.Join(zqkCache, ".tmp-durable-fresh")
	regularFile := filepath.Join(zqkCache, "regular.json")

	_ = fileutil.WriteFile(oldTmp, []byte("abandoned temp content"), 0o600)
	_ = fileutil.WriteFile(freshTmp, []byte("fresh temp content"), 0o600)
	_ = fileutil.WriteFile(regularFile, []byte("regular content"), 0o600)

	// Backdate oldTmp by 2 hours
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(oldTmp, oldTime, oldTime)

	// Dry run test
	cnt, bytes, reaped, err := ReapOrphanedTempFiles(tmpDir, 1*time.Hour, true)
	if err != nil {
		t.Fatalf("ReapOrphanedTempFiles dry run failed: %v", err)
	}
	if cnt != 1 || len(reaped) != 1 || bytes <= 0 {
		t.Errorf("expected 1 reaped dry run, got %d (bytes %d)", cnt, bytes)
	}
	if _, err := fileutil.Stat(oldTmp); err != nil {
		t.Errorf("oldTmp should still exist after dry run")
	}

	// Live run test
	cnt, bytes, reaped, err = ReapOrphanedTempFiles(tmpDir, 1*time.Hour, false)
	if err != nil {
		t.Fatalf("ReapOrphanedTempFiles failed: %v", err)
	}
	if cnt != 1 || len(reaped) != 1 || bytes <= 0 {
		t.Errorf("expected 1 reaped file, got %d (bytes %d)", cnt, bytes)
	}
	if _, err := fileutil.Stat(oldTmp); !fileutil.IsNotExist(err) {
		t.Errorf("oldTmp should have been deleted, but still exists")
	}
	if _, err := fileutil.Stat(freshTmp); err != nil {
		t.Errorf("freshTmp should not have been deleted: %v", err)
	}
	if _, err := fileutil.Stat(regularFile); err != nil {
		t.Errorf("regularFile should not have been deleted: %v", err)
	}
}

func TestReapStaleLocks(t *testing.T) {
	tmpDir := t.TempDir()
	zqkLocks := filepath.Join(tmpDir, paths.ProjectDataDir, "locks")
	if err := fileutil.MkdirAll(zqkLocks, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	staleLock := filepath.Join(zqkLocks, "test_stale.lock")
	activeLock := filepath.Join(zqkLocks, "test_active.lock")

	_ = fileutil.WriteFile(staleLock, []byte(""), 0o600)
	_ = fileutil.WriteFile(activeLock, []byte(""), 0o600)

	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(staleLock, oldTime, oldTime)
	_ = os.Chtimes(activeLock, oldTime, oldTime)

	// Hold active lock in this process
	activeF, errOpen := fileutil.OpenFile(activeLock, fileutil.O_RDWR, 0o600)
	if errOpen != nil {
		t.Fatalf("failed to open active lock: %v", errOpen)
	}
	defer activeF.Close()
	_ = syscallutil.FileFlock(activeF, syscall.LOCK_EX)

	cnt, reaped, err := ReapStaleLocks(tmpDir, 1*time.Hour, false)
	if err != nil {
		t.Fatalf("ReapStaleLocks failed: %v", err)
	}
	if cnt != 1 || len(reaped) != 1 {
		t.Errorf("expected exactly 1 stale lock reaped, got %d", cnt)
	}
	if _, err := fileutil.Stat(staleLock); !fileutil.IsNotExist(err) {
		t.Errorf("staleLock should be deleted, but exists")
	}
	if _, err := fileutil.Stat(activeLock); err != nil {
		t.Errorf("activeLock must NOT be deleted while held: %v", err)
	}
}

func TestEnforceLogRetention(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir)
	if err := fileutil.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	oldLog := filepath.Join(logDir, "old-events.log")
	oversizeLog := filepath.Join(logDir, "oversize.log")

	_ = fileutil.WriteFile(oldLog, []byte("old log entries"), 0o600)
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	_ = os.Chtimes(oldLog, oldTime, oldTime)

	// Create a 12,000-line log for truncation test
	var sb strings.Builder
	for i := 0; i < 12000; i++ {
		sb.WriteString("log entry line sample payload\n")
	}
	_ = fileutil.WriteFile(oversizeLog, []byte(sb.String()), 0o600)

	// Enforce retention: maxAge 7d, maxSizeBytes 10KB
	cnt, reclaimed, _, err := EnforceLogRetention(tmpDir, 7*24*time.Hour, 10*1024, false)
	if err != nil {
		t.Fatalf("EnforceLogRetention failed: %v", err)
	}
	if cnt < 2 || reclaimed <= 0 {
		t.Errorf("expected at least 2 pruned/truncated logs, got %d (reclaimed %d bytes)", cnt, reclaimed)
	}
	if _, err := fileutil.Stat(oldLog); !fileutil.IsNotExist(err) {
		t.Errorf("oldLog should have been deleted by age")
	}

	// Verify oversizeLog was truncated to 10000 lines
	data, errRead := fileutil.ReadFile(oversizeLog)
	if errRead != nil {
		t.Fatalf("failed to read oversizeLog: %v", errRead)
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) != 10000 {
		t.Errorf("expected 10000 lines after truncation, got %d", len(lines))
	}
}

func TestInspectIOResources(t *testing.T) {
	tmpDir := t.TempDir()
	zqkCache := filepath.Join(tmpDir, paths.ProjectDataDir, "cache")
	_ = fileutil.MkdirAll(zqkCache, 0o755)

	oldTmp := filepath.Join(zqkCache, ".tmp-durable-test")
	_ = fileutil.WriteFile(oldTmp, []byte("temp"), 0o600)
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(oldTmp, oldTime, oldTime)

	ctx := context.Background()
	telemetry, err := InspectIOResources(ctx, tmpDir)
	if err != nil {
		t.Fatalf("InspectIOResources failed: %v", err)
	}
	if telemetry == nil {
		t.Fatalf("expected non-nil telemetry")
	}
	if telemetry.OrphanedTempCount != 1 {
		t.Errorf("expected 1 orphaned temp detected, got %d", telemetry.OrphanedTempCount)
	}
}
