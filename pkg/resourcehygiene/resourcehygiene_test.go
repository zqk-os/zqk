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
	zqkCache := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir)
	if err := fileutil.MkdirAll(zqkCache, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	oldTmp := filepath.Join(zqkCache, ".tmp-durable-9999")
	freshTmp := filepath.Join(zqkCache, ".tmp-durable-fresh")
	regularFile := filepath.Join(zqkCache, "regular.json")

	_ = fileutil.WriteFile(oldTmp, []byte("abandoned temp content"), paths.FilePerm600)
	_ = fileutil.WriteFile(freshTmp, []byte("fresh temp content"), paths.FilePerm600)
	_ = fileutil.WriteFile(regularFile, []byte("regular content"), paths.FilePerm600)

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
	if err := fileutil.MkdirAll(zqkLocks, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	staleLock := filepath.Join(zqkLocks, "test_stale.lock")
	activeLock := filepath.Join(zqkLocks, "test_active.lock")

	_ = fileutil.WriteFile(staleLock, []byte(""), paths.FilePerm600)
	_ = fileutil.WriteFile(activeLock, []byte(""), paths.FilePerm600)

	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(staleLock, oldTime, oldTime)
	_ = os.Chtimes(activeLock, oldTime, oldTime)

	// Hold active lock in this process
	activeF, errOpen := fileutil.OpenFile(activeLock, fileutil.O_RDWR, paths.FilePerm600)
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
	if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	oldLog := filepath.Join(logDir, "old-events.log")
	oversizeLog := filepath.Join(logDir, "oversize.log")

	_ = fileutil.WriteFile(oldLog, []byte("old log entries"), paths.FilePerm600)
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	_ = os.Chtimes(oldLog, oldTime, oldTime)

	// Create a 12,000-line log for truncation test
	var sb strings.Builder
	for i := 0; i < 12000; i++ {
		sb.WriteString("log entry line sample payload\n")
	}
	_ = fileutil.WriteFile(oversizeLog, []byte(sb.String()), paths.FilePerm600)

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
	zqkCache := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir)
	_ = fileutil.MkdirAll(zqkCache, paths.DirPerm755)

	oldTmp := filepath.Join(zqkCache, ".tmp-durable-test")
	_ = fileutil.WriteFile(oldTmp, []byte("temp"), paths.FilePerm600)
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

func TestEnforceLogRetention_ProcessOutputsAndScheduler(t *testing.T) {
	tmpDir := t.TempDir()
	schedDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.SchedulerDir)
	logsDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir, "scheduler", "SCH-autofix")
	schedStateDir := filepath.Join(schedDir, "state")
	_ = fileutil.MkdirAll(schedDir, paths.DirPerm755)
	_ = fileutil.MkdirAll(logsDir, paths.DirPerm755)
	_ = fileutil.MkdirAll(schedStateDir, paths.DirPerm755)

	// Process outputs (.stdout and .stderr)
	stdoutLog := filepath.Join(logsDir, "job.stdout")
	stderrLog := filepath.Join(logsDir, "job.stderr")
	_ = fileutil.WriteFile(stdoutLog, []byte("stdout line 1\nstdout line 2\n"), paths.FilePerm600)
	_ = fileutil.WriteFile(stderrLog, []byte("stderr line 1\nstderr line 2\n"), paths.FilePerm600)
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	_ = os.Chtimes(stdoutLog, oldTime, oldTime)
	_ = os.Chtimes(stderrLog, oldTime, oldTime)

	// Scheduler diagnostic log in .zqk/scheduler
	schedDiag := filepath.Join(schedDir, "diagnostics-20260920.jsonl")
	_ = fileutil.WriteFile(schedDiag, []byte("{\"diag\":1}\n"), paths.FilePerm600)
	_ = os.Chtimes(schedDiag, oldTime, oldTime)

	// Protected scheduler state files
	issuesFile := filepath.Join(schedDir, "issues.json")
	summaryFile := filepath.Join(schedDir, "scheduler-metrics-summary.json")
	stateFile := filepath.Join(schedStateDir, "state.json")
	_ = fileutil.WriteFile(issuesFile, []byte("{\"issues\":[]}"), paths.FilePerm600)
	_ = fileutil.WriteFile(summaryFile, []byte("{\"metrics\":{}}"), paths.FilePerm600)
	_ = fileutil.WriteFile(stateFile, []byte("{\"state\":\"ok\"}"), paths.FilePerm600)
	_ = os.Chtimes(issuesFile, oldTime, oldTime)
	_ = os.Chtimes(summaryFile, oldTime, oldTime)
	_ = os.Chtimes(stateFile, oldTime, oldTime)

	// Run retention: maxAge 7d
	cnt, reclaimed, _, err := EnforceLogRetention(tmpDir, 7*24*time.Hour, 10*1024*1024, false)
	if err != nil {
		t.Fatalf("EnforceLogRetention failed: %v", err)
	}
	if cnt != 3 {
		t.Errorf("expected 3 pruned logs (.stdout, .stderr, .jsonl), got %d (reclaimed %d bytes)", cnt, reclaimed)
	}

	// Verify pruned
	if _, err := fileutil.Stat(stdoutLog); !fileutil.IsNotExist(err) {
		t.Errorf("expected stdoutLog to be deleted")
	}
	if _, err := fileutil.Stat(stderrLog); !fileutil.IsNotExist(err) {
		t.Errorf("expected stderrLog to be deleted")
	}
	if _, err := fileutil.Stat(schedDiag); !fileutil.IsNotExist(err) {
		t.Errorf("expected schedDiag to be deleted")
	}

	// Verify protected
	if _, err := fileutil.Stat(issuesFile); err != nil {
		t.Errorf("issuesFile should NOT be deleted: %v", err)
	}
	if _, err := fileutil.Stat(summaryFile); err != nil {
		t.Errorf("summaryFile should NOT be deleted: %v", err)
	}
	if _, err := fileutil.Stat(stateFile); err != nil {
		t.Errorf("stateFile in state dir should NOT be deleted: %v", err)
	}
}

func TestInspectIOResources_HeldLockNotReportedStale(t *testing.T) {
	tmpDir := t.TempDir()
	zqkLocks := filepath.Join(tmpDir, paths.ProjectDataDir, "locks")
	if err := fileutil.MkdirAll(zqkLocks, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	staleLock := filepath.Join(zqkLocks, "unheld.lock")
	activeLock := filepath.Join(zqkLocks, "held.lock")
	_ = fileutil.WriteFile(staleLock, []byte(""), paths.FilePerm600)
	_ = fileutil.WriteFile(activeLock, []byte(""), paths.FilePerm600)

	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(staleLock, oldTime, oldTime)
	_ = os.Chtimes(activeLock, oldTime, oldTime)

	// Actively hold flock on activeLock
	f, errOpen := fileutil.OpenFile(activeLock, fileutil.O_RDWR, paths.FilePerm600)
	if errOpen != nil {
		t.Fatalf("failed to open active lock: %v", errOpen)
	}
	defer f.Close()
	if err := syscallutil.FileFlock(f, syscall.LOCK_EX); err != nil {
		t.Fatalf("failed to flock: %v", err)
	}

	report, err := InspectIOResources(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("InspectIOResources failed: %v", err)
	}
	if report.StaleLocksCount != 1 {
		t.Fatalf("expected exactly 1 stale lock, got %d (%v)", report.StaleLocksCount, report.StaleLockPaths)
	}
	if !strings.Contains(report.StaleLockPaths[0], "unheld.lock") {
		t.Fatalf("expected unheld.lock in StaleLockPaths, got %v", report.StaleLockPaths)
	}
}

func TestInspectIOResources_DaemonLockNeverReportedStale(t *testing.T) {
	tmpDir := t.TempDir()
	daemonLocks := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "daemon_locks")
	if err := fileutil.MkdirAll(daemonLocks, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	ambientLock := filepath.Join(daemonLocks, "ambient.lock")
	_ = fileutil.WriteFile(ambientLock, []byte(""), paths.FilePerm600)
	oldTime := time.Now().Add(-24 * time.Hour)
	_ = os.Chtimes(ambientLock, oldTime, oldTime)

	report, err := InspectIOResources(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("InspectIOResources failed: %v", err)
	}
	if report.StaleLocksCount != 0 {
		t.Fatalf("expected 0 stale locks for daemon singleton locks, got %d (%v)", report.StaleLocksCount, report.StaleLockPaths)
	}
}

func TestReapStaleLocks_DaemonLockNeverReaped(t *testing.T) {
	tmpDir := t.TempDir()
	daemonLocks := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "daemon_locks")
	if err := fileutil.MkdirAll(daemonLocks, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	ambientLock := filepath.Join(daemonLocks, "ambient.lock")
	_ = fileutil.WriteFile(ambientLock, []byte(""), paths.FilePerm600)
	oldTime := time.Now().Add(-24 * time.Hour)
	_ = os.Chtimes(ambientLock, oldTime, oldTime)

	cnt, _, err := ReapStaleLocks(tmpDir, 1*time.Hour, false)
	if err != nil {
		t.Fatalf("ReapStaleLocks failed: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("expected 0 locks reaped, got %d", cnt)
	}
	if _, err := fileutil.Stat(ambientLock); err != nil {
		t.Fatalf("daemon lock must not be reaped: %v", err)
	}
}

func TestReapOrphanedProcesses(t *testing.T) {
	tmpDir := t.TempDir()

	origList := listProcessesHook
	origCmd := getCommandLineHook
	origKill := killProcessHook
	origAlive := isProcessAliveHook
	defer func() {
		listProcessesHook = origList
		getCommandLineHook = origCmd
		killProcessHook = origKill
		isProcessAliveHook = origAlive
	}()

	killedSignals := make(map[int][]syscall.Signal)
	aliveMap := map[int]bool{9999: true, 9998: true}

	listProcessesHook = func() ([]procEntry, error) {
		return []procEntry{
			{pid: 1, ppid: 0, exe: "init"},
			{pid: os.Getpid(), ppid: 1, exe: "zqk"}, // self PID must be skipped
			{pid: 9999, ppid: 1, exe: "zqk"},        // orphan belonging to project
			{pid: 9998, ppid: 1, exe: "zqk"},        // orphan belonging to other project
			{pid: 9997, ppid: 1234, exe: "zqk"},     // not orphan (ppid != 1)
			{pid: 9996, ppid: 1, exe: "bash"},       // not zqk executable
		}, nil
	}

	getCommandLineHook = func(pid int) (string, error) {
		if pid == 9999 {
			return "zqk orchestrate " + tmpDir, nil
		}
		if pid == 9998 {
			return "zqk orchestrate /other/project", nil
		}
		return "", nil
	}

	killProcessHook = func(pid int, sig syscall.Signal) error {
		killedSignals[pid] = append(killedSignals[pid], sig)
		if sig == syscall.SIGKILL {
			aliveMap[pid] = false
		}
		return nil
	}

	isProcessAliveHook = func(pid int) bool {
		return aliveMap[pid]
	}

	// 1. Dry run
	countDry, pathsDry, err := ReapOrphanedProcesses(tmpDir, true)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if countDry != 1 {
		t.Fatalf("expected 1 orphan reported in dry run, got %d", countDry)
	}
	if len(killedSignals) != 0 {
		t.Fatalf("expected no kills during dry run, got %v", killedSignals)
	}
	if !strings.Contains(pathsDry[0], "9999") {
		t.Fatalf("expected PID 9999 in dry run paths: %v", pathsDry)
	}

	// 2. Live run
	countLive, pathsLive, err := ReapOrphanedProcesses(tmpDir, false)
	if err != nil {
		t.Fatalf("live run failed: %v", err)
	}
	if countLive != 1 {
		t.Fatalf("expected 1 orphan reaped in live run, got %d", countLive)
	}
	if len(killedSignals[9999]) < 1 {
		t.Fatalf("expected signals sent to 9999, got %v", killedSignals[9999])
	}
	if !strings.Contains(pathsLive[0], "terminated") {
		t.Fatalf("expected terminated message in live paths: %v", pathsLive)
	}
}

