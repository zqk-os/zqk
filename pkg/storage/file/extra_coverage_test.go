package file

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraCoverage_FileLockMetrics(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	ResetFileLockMetrics()
	m := GetFileLockMetrics()

	// Initial metrics with 0 operations
	initSnap := m.GetSnapshot()
	if ops := initSnap.GetTotalOperations(); ops != 0 {
		t.Errorf("expected 0 total operations, got %d", ops)
	}
	if avg := initSnap.AverageAcquisitionTime(); avg != 0 {
		t.Errorf("expected 0 average acquisition time, got %v", avg)
	}
	if avg := initSnap.AverageWaitTime(); avg != 0 {
		t.Errorf("expected 0 average wait time, got %v", avg)
	}
	if rate := initSnap.ContentionRate(); rate != 0 {
		t.Errorf("expected 0 contention rate, got %v", rate)
	}
	if rate := initSnap.SuccessRate(); rate != 0 {
		t.Errorf("expected 0 success rate, got %v", rate)
	}

	// Record operations
	m.RecordAcquisition(10 * time.Millisecond)
	m.RecordAcquisition(20 * time.Millisecond)
	m.RecordFailure()
	m.RecordTimeout(15 * time.Millisecond)
	m.RecordContention()
	m.RecordWaitTime(5 * time.Millisecond)
	m.IncrementHolders()
	m.IncrementHolders()
	m.DecrementHolders()
	m.RecordContentionAttempt()

	snap := m.GetSnapshot()
	if snap.TotalAcquisitions != 2 {
		t.Errorf("expected 2 acquisitions, got %d", snap.TotalAcquisitions)
	}
	if snap.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", snap.TotalFailures)
	}
	if snap.TotalTimeouts != 1 {
		t.Errorf("expected 1 timeout, got %d", snap.TotalTimeouts)
	}
	if snap.CurrentHolders != 1 {
		t.Errorf("expected 1 current holder, got %d", snap.CurrentHolders)
	}

	if snap.GetTotalOperations() == 0 {
		t.Error("expected non-zero total operations")
	}
	if snap.AverageAcquisitionTime() == 0 {
		t.Error("expected non-zero average acquisition time")
	}
	if snap.AverageWaitTime() == 0 {
		t.Error("expected non-zero average wait time")
	}
	if snap.ContentionRate() == 0 {
		t.Error("expected non-zero contention rate")
	}
	if snap.SuccessRate() == 0 {
		t.Error("expected non-zero success rate")
	}
}

func TestExtraCoverage_FileLockStrategyMetrics(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	ResetFileLockStrategyMetrics()
	sm := GetFileLockStrategyMetrics()

	// Initial metrics
	initSnap := sm.GetSnapshot()
	if ops := initSnap.GetTotalOperations(); ops != 0 {
		t.Errorf("expected 0 total operations, got %d", ops)
	}
	if rate := initSnap.GetContentionRate(); rate != 0 {
		t.Errorf("expected 0 contention rate, got %v", rate)
	}

	// Record strategy operations
	sm.RecordAcquisition("persistent", "index", 5*time.Millisecond)
	sm.RecordAcquisition("persistent", "cache", 10*time.Millisecond)
	sm.RecordFailure("persistent", "file")
	sm.RecordTimeout("persistent", "system_object")
	sm.RecordStaleCleanup(3 * time.Millisecond)

	snap := sm.GetSnapshot()
	if len(snap.StrategyAcquisitions) == 0 {
		t.Error("expected strategy acquisitions to be recorded")
	}

	breakdown := snap.GetResourceTypeBreakdown()
	if len(breakdown) == 0 {
		t.Error("expected resource type breakdown")
	}

	events, rTypes := sm.GetFileLockStrategyTotalStats()
	if events < 2 {
		t.Errorf("expected at least 2 events in total stats, got %d", events)
	}
	if rTypes < 1 {
		t.Errorf("expected at least 1 resource type in total stats, got %d", rTypes)
	}
	var nilMetrics *FileLockStrategyMetrics
	if e, r := nilMetrics.GetFileLockStrategyTotalStats(); e != 0 || r != 0 {
		t.Errorf("expected (0, 0) for nil metrics, got (%d, %d)", e, r)
	}
}

func TestExtraCoverage_PersistentLockStrategy(t *testing.T) {
	strat := NewPersistentLockStrategy()
	strat.StaleLockThreshold = 10 * time.Millisecond
	if name := strat.Name(); name != "persistent" {
		t.Errorf("expected strategy name persistent, got %s", name)
	}

	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Cleanup non-existent lock
	if err := strat.CleanupStaleLocks(lockPath); err != nil {
		t.Errorf("expected nil for non-existent lock, got %v", err)
	}

	// Cleanup actual stale lock
	stalePath := filepath.Join(tmpDir, "stale_pers.lock")
	_ = fileutil.WriteFile(stalePath, []byte("stale"), paths.FilePerm644)
	time.Sleep(25 * time.Millisecond)
	if err := strat.CleanupStaleLocks(stalePath); err != nil {
		t.Errorf("CleanupStaleLocks on stale file: %v", err)
	}

	// Acquire lock and release
	handle, err := strat.AcquireLock(lockPath, 1*time.Second)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if !handle.IsLocked() {
		t.Error("expected handle to be locked")
	}
	if err := handle.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Test determineResourceType paths
	testCases := []struct {
		path     string
		expected string
	}{
		{filepath.Join(tmpDir, "meta.index.lock"), "index"},
		{filepath.Join(tmpDir, "cache", "item.lock"), "cache"},
		{filepath.Join(tmpDir, paths.ProjectDataDir, "item.lock"), "system_object"},
		{filepath.Join(tmpDir, "process", "item.lock"), "public_object"},
		{filepath.Join(tmpDir, "plain.lock"), "file"},
	}
	for _, tc := range testCases {
		if res := strat.determineResourceType(tc.path); res != tc.expected {
			t.Errorf("path %s: expected %s, got %s", tc.path, tc.expected, res)
		}
	}

	// WithFileLock
	executed := false
	err = WithFileLock(strat, filepath.Join(tmpDir, "with_lock.lock"), 1*time.Second, func() error {
		executed = true
		return nil
	})
	if err != nil || !executed {
		t.Errorf("WithFileLock failed: %v executed=%v", err, executed)
	}

	// WithFileLock error propagation
	expectedErr := errors.New("inner error")
	err = WithFileLock(strat, filepath.Join(tmpDir, "with_lock2.lock"), 1*time.Second, func() error {
		return expectedErr
	})
	if err == nil || !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestExtraCoverage_ShardedLockStrategy_Methods(t *testing.T) {
	tmpDir := t.TempDir()
	strat := NewShardedFileLockStrategy(0, filepath.Join(tmpDir, "locks"))
	if name := strat.Name(); name != "sharded-file-lock" {
		t.Errorf("expected sharded-file-lock, got %s", name)
	}

	lockPath := filepath.Join(tmpDir, "direct.lock")
	handle, err := strat.AcquireLock(lockPath, 1*time.Second)
	if err != nil {
		t.Fatalf("AcquireLock direct: %v", err)
	}
	_ = handle.Release()

	if err := strat.CleanupStaleLocks(lockPath); err != nil {
		t.Errorf("CleanupStaleLocks direct: %v", err)
	}

	// WithShardedFileLock with nil strategy
	ran := false
	_ = WithShardedFileLock(nil, "key1", 1*time.Second, func() error {
		ran = true
		return nil
	})
	if !ran {
		t.Error("expected nil strategy to run fn")
	}
}

func TestExtraCoverage_AutoCleanupStrategy_Threshold(t *testing.T) {
	strat := NewAutoCleanupStrategyWithThreshold(10 * time.Millisecond)
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "stale.lock")

	_ = fileutil.WriteFile(lockPath, []byte("stale"), paths.FilePerm644)
	time.Sleep(25 * time.Millisecond)

	if err := strat.CleanupStaleLocks(lockPath); err != nil {
		t.Errorf("CleanupStaleLocks: %v", err)
	}
	if fileutil.Exists(lockPath) {
		t.Error("expected stale lock to be deleted")
	}
}

func TestExtraCoverage_FileStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	relPath := filepath.Join("nested", "item.txt")
	content := []byte("stored data")
	if err := store.Write(relPath, content); err != nil {
		t.Fatalf("store.Write: %v", err)
	}

	read, err := store.Read(relPath)
	if err != nil {
		t.Fatalf("store.Read: %v", err)
	}
	if string(read) != string(content) {
		t.Errorf("expected %s, got %s", content, read)
	}
}

func TestExtraCoverage_GlobalContentionHistogram(t *testing.T) {
	hist := GetGlobalContentionHistogram()
	if hist == nil {
		t.Fatal("expected non-nil global histogram")
	}
	hist.Record(2 * time.Millisecond)
	snap := hist.Snapshot()
	if snap.TotalSamples == 0 {
		t.Error("expected non-zero total samples")
	}
}

func TestExtraCoverage_FileLockWithLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "withlock.lock")
	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}
	defer fl.Close()

	ran := false
	err = fl.WithLock(func() error {
		ran = true
		return nil
	})
	if err != nil || !ran {
		t.Errorf("WithLock: err=%v ran=%v", err, ran)
	}

	ranTimeout := false
	err = fl.WithLockTimeout(1*time.Second, func() error {
		ranTimeout = true
		return nil
	})
	if err != nil || !ranTimeout {
		t.Errorf("WithLockTimeout: err=%v ran=%v", err, ranTimeout)
	}
}
