package resourcehygiene

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDefaultHygieneOptions(t *testing.T) {
	t.Parallel()
	opts := DefaultHygieneOptions()
	if !opts.ReapLocks || !opts.ReapTemp || !opts.EnforceRetention {
		t.Errorf("expected all actions enabled by default in HygieneOptions, got %+v", opts)
	}
	if opts.DryRun {
		t.Errorf("expected DryRun to be false by default")
	}
	if opts.LockThreshold <= 0 || opts.TempThreshold <= 0 || opts.LogMaxAge <= 0 || opts.LogMaxSize <= 0 {
		t.Errorf("expected positive duration and size defaults in HygieneOptions, got %+v", opts)
	}
}

func TestReapEmptyProjectRootSafeties(t *testing.T) {
	t.Parallel()

	// 1. Empty projectRoot
	cnt, bytes, reaped, err := ReapOrphanedTempFiles("", 0, false)
	if err != nil || cnt != 0 || bytes != 0 || reaped != nil {
		t.Errorf("expected zeroes for empty projectRoot in ReapOrphanedTempFiles")
	}

	cnt2, reaped2, err2 := ReapStaleLocks("", 0, false)
	if err2 != nil || cnt2 != 0 || reaped2 != nil {
		t.Errorf("expected zeroes for empty projectRoot in ReapStaleLocks")
	}

	cnt3, bytes3, reaped3, err3 := EnforceLogRetention("", 0, 0, false)
	if err3 != nil || cnt3 != 0 || bytes3 != 0 || reaped3 != nil {
		t.Errorf("expected zeroes for empty projectRoot in EnforceLogRetention")
	}

	// 2. Default fallback thresholds (<= 0)
	tmpDir := t.TempDir()
	_, _, _, err = ReapOrphanedTempFiles(tmpDir, -1, true)
	if err != nil {
		t.Errorf("ReapOrphanedTempFiles fallback threshold failed: %v", err)
	}

	_, _, err = ReapStaleLocks(tmpDir, -1, true)
	if err != nil {
		t.Errorf("ReapStaleLocks fallback threshold failed: %v", err)
	}

	_, _, _, err = EnforceLogRetention(tmpDir, -1, -1, true)
	if err != nil {
		t.Errorf("EnforceLogRetention fallback threshold failed: %v", err)
	}
}

func TestInspectIOResources_EmptyRootAndAnomalies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Empty root
	telemetry, err := InspectIOResources(ctx, "")
	if err != nil {
		t.Fatalf("InspectIOResources empty root failed: %v", err)
	}
	if telemetry == nil {
		t.Fatalf("expected non-nil telemetry on empty root")
	}

	// With stale lock and temp files
	root := t.TempDir()
	zqkDir := filepath.Join(root, paths.ProjectDataDir)
	_ = fileutil.MkdirAll(zqkDir, paths.DirPerm750)

	staleLock := filepath.Join(zqkDir, "old.lock")
	_ = fileutil.WriteFile(staleLock, []byte(""), paths.FilePerm600)
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = fileutil.Chtimes(staleLock, oldTime, oldTime)

	oldTemp := filepath.Join(zqkDir, ".tmp-orphaned-999")
	_ = fileutil.WriteFile(oldTemp, []byte("temp"), paths.FilePerm600)
	_ = fileutil.Chtimes(oldTemp, oldTime, oldTime)

	telemetry2, err := InspectIOResources(ctx, root)
	if err != nil {
		t.Fatalf("InspectIOResources failed: %v", err)
	}
	if telemetry2.StaleLocksCount < 1 {
		t.Errorf("expected at least 1 stale lock detected, got %d", telemetry2.StaleLocksCount)
	}
	if telemetry2.OrphanedTempCount < 1 {
		t.Errorf("expected at least 1 orphaned temp detected, got %d", telemetry2.OrphanedTempCount)
	}
}

func TestExecuteHygiene_DryRunAndLive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Setup structure
	zqkDir := filepath.Join(root, paths.ProjectDataDir)
	cacheDir := filepath.Join(zqkDir, paths.CacheDir)
	locksDir := filepath.Join(zqkDir, "locks")
	logsDir := filepath.Join(zqkDir, paths.LogsDir)
	_ = fileutil.MkdirAll(cacheDir, paths.DirPerm750)
	_ = fileutil.MkdirAll(locksDir, paths.DirPerm750)
	_ = fileutil.MkdirAll(logsDir, paths.DirPerm750)

	oldTime := time.Now().Add(-48 * time.Hour)

	// Temp file
	tempFile := filepath.Join(cacheDir, ".tmp-sweep-1")
	_ = fileutil.WriteFile(tempFile, []byte("sweep-temp"), paths.FilePerm600)
	_ = fileutil.Chtimes(tempFile, oldTime, oldTime)

	// Stale lock
	staleLock := filepath.Join(locksDir, "sweep.lock")
	_ = fileutil.WriteFile(staleLock, []byte(""), paths.FilePerm600)
	_ = fileutil.Chtimes(staleLock, oldTime, oldTime)

	// Aged log
	agedLog := filepath.Join(logsDir, "sweep-old.log")
	_ = fileutil.WriteFile(agedLog, []byte("old log entries\n"), paths.FilePerm600)
	_ = fileutil.Chtimes(agedLog, oldTime, oldTime)

	// 1. Dry run
	opts := HygieneOptions{
		ReapLocks:        true,
		ReapTemp:         true,
		EnforceRetention: true,
		DryRun:           true,
		LockThreshold:    1 * time.Hour,
		TempThreshold:    1 * time.Hour,
		LogMaxAge:        24 * time.Hour,
		LogMaxSize:       10 * 1024 * 1024,
	}

	reportDry, err := ExecuteHygiene(root, opts)
	if err != nil {
		t.Fatalf("ExecuteHygiene dry run failed: %v", err)
	}
	if !reportDry.DryRun {
		t.Errorf("expected DryRun=true in report")
	}
	if reportDry.TempReaped != 1 || reportDry.LocksReaped != 1 || reportDry.LogsPruned != 1 {
		t.Errorf("expected 1 of each reaped in dry run, got temp=%d, locks=%d, logs=%d",
			reportDry.TempReaped, reportDry.LocksReaped, reportDry.LogsPruned)
	}
	if len(reportDry.ReapedPaths) < 3 {
		t.Errorf("expected at least 3 reaped paths in dry run, got %d", len(reportDry.ReapedPaths))
	}

	// Verify files still exist after dry run
	if _, err := fileutil.Stat(tempFile); err != nil {
		t.Errorf("tempFile was deleted during dry run: %v", err)
	}
	if _, err := fileutil.Stat(staleLock); err != nil {
		t.Errorf("staleLock was deleted during dry run: %v", err)
	}
	if _, err := fileutil.Stat(agedLog); err != nil {
		t.Errorf("agedLog was deleted during dry run: %v", err)
	}

	// 2. Live run
	opts.DryRun = false
	reportLive, err := ExecuteHygiene(root, opts)
	if err != nil {
		t.Fatalf("ExecuteHygiene live run failed: %v", err)
	}
	if reportLive.DryRun {
		t.Errorf("expected DryRun=false in live report")
	}
	if reportLive.TempReaped != 1 || reportLive.LocksReaped != 1 || reportLive.LogsPruned != 1 {
		t.Errorf("expected 1 of each reaped in live run, got temp=%d, locks=%d, logs=%d",
			reportLive.TempReaped, reportLive.LocksReaped, reportLive.LogsPruned)
	}

	// Verify files were reaped in live run
	if _, err := fileutil.Stat(tempFile); !fileutil.IsNotExist(err) {
		t.Errorf("tempFile still exists after live run")
	}
	if _, err := fileutil.Stat(staleLock); !fileutil.IsNotExist(err) {
		t.Errorf("staleLock still exists after live run")
	}
	if _, err := fileutil.Stat(agedLog); !fileutil.IsNotExist(err) {
		t.Errorf("agedLog still exists after live run")
	}
}

func TestEnforceLogRetention_NonExistentLogDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// No logs directory created under root
	cnt, reclaimed, paths, err := EnforceLogRetention(root, 24*time.Hour, 1024, false)
	if err != nil {
		t.Fatalf("EnforceLogRetention on empty root failed: %v", err)
	}
	if cnt != 0 || reclaimed != 0 || len(paths) != 0 {
		t.Errorf("expected 0 pruned for non-existent log dirs, got %d", cnt)
	}
}
