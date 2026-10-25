// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package resourcehygiene

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

func setModTime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := fileutil.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("failed to set modtime on %s: %v", path, err)
	}
}

func createTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to mkdir %s: %v", dir, err)
	}
	if err := fileutil.WriteFile(path, content, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func TestCycle201_TypesAndSerialization(t *testing.T) {
	t.Parallel()
	opts := DefaultHygieneOptions()
	if !opts.ReapScorecards || !opts.ReapMetrics || !opts.ReapProcesses {
		t.Errorf("expected scorecards, metrics, and processes enabled by default")
	}

	optsBytes, err := json.Marshal(opts)
	if err != nil {
		t.Fatalf("failed to marshal HygieneOptions: %v", err)
	}
	var unmarshaledOpts HygieneOptions
	if err := json.Unmarshal(optsBytes, &unmarshaledOpts); err != nil {
		t.Fatalf("failed to unmarshal HygieneOptions: %v", err)
	}
	if unmarshaledOpts.LockThreshold != opts.LockThreshold {
		t.Errorf("lock threshold mismatch: %v vs %v", unmarshaledOpts.LockThreshold, opts.LockThreshold)
	}

	report := &HygieneExecutionReport{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		DryRun:           true,
		LocksReaped:      2,
		TempReaped:       3,
		ProcessesReaped:  1,
		LogsPruned:       4,
		ScorecardsReaped: 5,
		MetricsReaped:    6,
		BytesReclaimed:   1024,
		ReapedPaths:      []string{"a.tmp", "b.lock"},
		Errors:           []string{"test warning"},
	}
	repBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}
	var unmarshaledRep HygieneExecutionReport
	if err := json.Unmarshal(repBytes, &unmarshaledRep); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}
	if unmarshaledRep.LocksReaped != 2 || unmarshaledRep.ScorecardsReaped != 5 {
		t.Errorf("report count mismatch: %+v", unmarshaledRep)
	}

	telemetry := &IOResourceTelemetry{
		CapturedAt:           time.Now().UTC().Format(time.RFC3339),
		OpenFileDescriptors:  10,
		MaxFileDescriptors:   1024,
		TotalZqkFiles:        50,
		TotalZqkBytes:        4096,
		VolumeTotalBytes:     100000,
		VolumeAvailableBytes: 50000,
		ZqkChildren: map[string]ChildFolderUsage{
			"logs": {Files: 10, Bytes: 2048},
		},
		StaleLocksCount:      1,
		OrphanedTempCount:    2,
		OrphanedProcessCount: 1,
		StaleLockPaths:       []string{"locks/old.lock"},
		OrphanedTempPaths:    []string{"cache/.tmp-1"},
		OrphanedProcesses:    []string{"PID 1234: zqk"},
	}
	telBytes, err := json.Marshal(telemetry)
	if err != nil {
		t.Fatalf("failed to marshal telemetry: %v", err)
	}
	var unmarshaledTel IOResourceTelemetry
	if err := json.Unmarshal(telBytes, &unmarshaledTel); err != nil {
		t.Fatalf("failed to unmarshal telemetry: %v", err)
	}
	if unmarshaledTel.OpenFileDescriptors != 10 || len(unmarshaledTel.ZqkChildren) != 1 {
		t.Errorf("telemetry field mismatch: %+v", unmarshaledTel)
	}
}

func TestCycle201_DefaultHooksExercise(t *testing.T) {
	procs, err := listProcessesHook()
	if err != nil {
		t.Logf("listProcessesHook returned non-nil err (expected in some sandbox envs): %v", err)
	} else if len(procs) == 0 {
		t.Errorf("expected at least one system process")
	}

	selfPid := 1
	cmdLine, err := getCommandLineHook(selfPid)
	if err != nil {
		t.Logf("getCommandLineHook returned error: %v", err)
	} else {
		t.Logf("getCommandLineHook returned: %s", cmdLine)
	}

	bogusPID := 999999999
	if errKill := killProcessHook(bogusPID, syscall.Signal(0)); errKill != nil {
		t.Logf("killProcessHook expected error on bogus PID: %v", errKill)
	}
	if isProcessAliveHook(bogusPID) {
		t.Errorf("bogus process should not be alive")
	}
}

func TestCycle201_ReapOrphanedProcesses_EdgeCases(t *testing.T) {
	cnt, pathsList, err := ReapOrphanedProcesses("   ", false)
	if err != nil || cnt != 0 || len(pathsList) != 0 {
		t.Fatalf("expected zeroes for whitespace projectRoot: %d, %v, %v", cnt, pathsList, err)
	}

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

	listProcessesHook = func() ([]procEntry, error) {
		return nil, errors.New("simulated process enumeration failure")
	}
	_, _, err = ReapOrphanedProcesses(t.TempDir(), false)
	if err == nil {
		t.Fatalf("expected error when listProcessesHook fails")
	}

	tmpDir := t.TempDir()
	daemonLockPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "daemon_locks", "steward.lock")
	createTestFile(t, daemonLockPath, []byte(""))
	daemonF, errDaemon := fileutil.OpenFile(daemonLockPath, fileutil.O_RDWR, paths.FilePerm600)
	if errDaemon == nil {
		defer daemonF.Close()
		if errFlock := syscallutil.FileFlock(daemonF, syscall.LOCK_EX); errFlock != nil {
			t.Logf("daemon lock flock failed: %v", errFlock)
		}
	}

	parentPid := syscall.Getppid()
	listProcessesHook = func() ([]procEntry, error) {
		return []procEntry{
			{pid: 2001, ppid: 2001, exe: "zqk"},
			{pid: 2002, ppid: 1, exe: "zqk"},
			{pid: 2003, ppid: 1, exe: "zqk"},
			{pid: 2004, ppid: 1, exe: "not-zqk-binary"},
			{pid: syscall.Getpid(), ppid: 1, exe: "zqk"},
			{pid: parentPid, ppid: 1, exe: "zqk"},
		}, nil
	}
	getCommandLineHook = func(pid int) (string, error) {
		if pid == 2002 {
			return "", errors.New("cmdline unavailable")
		}
		if pid == 2003 {
			return "zqk orchestrate --root=/different/workspace", nil
		}
		return "", nil
	}
	killProcessHook = func(pid int, sig syscall.Signal) error {
		return nil
	}
	isProcessAliveHook = func(pid int) bool {
		return false
	}

	cnt, reaped, err := ReapOrphanedProcesses(tmpDir, false)
	if err != nil {
		t.Fatalf("unexpected error in edge cases: %v", err)
	}
	if cnt != 0 || len(reaped) != 0 {
		t.Fatalf("expected 0 orphans reaped, got %d (%v)", cnt, reaped)
	}
}

func TestCycle201_ReapOrphanedScorecards_FullCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	cleanRoot := filepath.Clean(tmpDir)
	scorecardsDir := filepath.Join(cleanRoot, paths.ProjectDataDir, "scorecards")

	if err := fileutil.MkdirAll(scorecardsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	subDir := filepath.Join(scorecardsDir, "nested_dir")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir subDir failed: %v", err)
	}

	ignoredFile := filepath.Join(scorecardsDir, "readme.txt")
	createTestFile(t, ignoredFile, []byte("ignored non-json"))

	liveSrc := filepath.Join(cleanRoot, "pkg", "actual", "component.go")
	createTestFile(t, liveSrc, []byte("package actual"))

	scWithCustomPath := filepath.Join(scorecardsDir, "random__encoded__name.json")
	scPayload := fmt.Sprintf(`{"file_path": "pkg/actual/component.go"}`)
	createTestFile(t, scWithCustomPath, []byte(scPayload))

	orphanSC := filepath.Join(scorecardsDir, "missing__target.json")
	createTestFile(t, orphanSC, []byte(`{"file_path": "pkg/missing/target.go"}`))

	cnt, bytes, reaped, err := ReapOrphanedScorecards(cleanRoot, true)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if cnt != 1 || len(reaped) != 1 || bytes <= 0 {
		t.Fatalf("expected 1 orphan in dry run, got %d (bytes: %d)", cnt, bytes)
	}

	cntLive, _, _, err := ReapOrphanedScorecards(cleanRoot, false)
	if err != nil {
		t.Fatalf("live run failed: %v", err)
	}
	if cntLive != 1 {
		t.Fatalf("expected 1 orphan reaped in live run, got %d", cntLive)
	}
	if _, err := fileutil.Stat(orphanSC); !fileutil.IsNotExist(err) {
		t.Fatalf("expected orphan scorecard to be removed")
	}
	if _, err := fileutil.Stat(scWithCustomPath); err != nil {
		t.Fatalf("valid scorecard matching custom file_path must not be removed: %v", err)
	}
}

func TestCycle201_ReapAgedMetrics_FullCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	cleanRoot := filepath.Clean(tmpDir)

	cnt0, _, _, err := ReapAgedMetrics(cleanRoot, 0, false)
	if err != nil {
		t.Fatalf("reap with default maxAge failed: %v", err)
	}
	if cnt0 != 0 {
		t.Fatalf("expected 0 for empty metrics dir")
	}

	cmdDir := filepath.Join(cleanRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsCommandMetricsSubdir)
	nestedDir := filepath.Join(cmdDir, "nested_dir")
	if err := fileutil.MkdirAll(nestedDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir nested failed: %v", err)
	}

	nonMetric := filepath.Join(cmdDir, "info.txt")
	createTestFile(t, nonMetric, []byte("non-metric file"))

	agedChunk := filepath.Join(cmdDir, "sample.chunk")
	createTestFile(t, agedChunk, []byte("chunk payload"))
	agedJSON := filepath.Join(cmdDir, "sample.json")
	createTestFile(t, agedJSON, []byte("json payload"))
	agedTmp := filepath.Join(cmdDir, "sample.tmp")
	createTestFile(t, agedTmp, []byte("tmp payload"))

	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	setModTime(t, agedChunk, oldTime)
	setModTime(t, agedJSON, oldTime)
	setModTime(t, agedTmp, oldTime)

	cnt, bytes, reaped, err := ReapAgedMetrics(cleanRoot, 14*24*time.Hour, true)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if cnt != 3 || len(reaped) != 3 || bytes <= 0 {
		t.Fatalf("expected 3 aged metrics in dry run, got %d (bytes %d)", cnt, bytes)
	}

	cntLive, _, _, err := ReapAgedMetrics(cleanRoot, 14*24*time.Hour, false)
	if err != nil {
		t.Fatalf("live run failed: %v", err)
	}
	if cntLive != 3 {
		t.Fatalf("expected 3 live reaped metrics, got %d", cntLive)
	}
	if _, err := fileutil.Stat(nonMetric); err != nil {
		t.Fatalf("nonMetric file must be preserved: %v", err)
	}
}

func TestCycle201_EnforceLogRetention_TruncationVariants(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir)

	mediumLog := filepath.Join(logDir, "medium.log")
	var mediumLines []string
	for i := 0; i < 300; i++ {
		mediumLines = append(mediumLines, fmt.Sprintf("line %03d of medium log", i))
	}
	createTestFile(t, mediumLog, []byte(strings.Join(mediumLines, "\n")))

	tinyLog := filepath.Join(logDir, "tiny.log")
	var tinyLines []string
	for i := 0; i < 10; i++ {
		tinyLines = append(tinyLines, fmt.Sprintf("short %d", i))
	}
	createTestFile(t, tinyLog, []byte(strings.Join(tinyLines, "\n")))

	cnt, reclaimed, reaped, err := EnforceLogRetention(tmpDir, 48*time.Hour, 50, false)
	if err != nil {
		t.Fatalf("truncation test failed: %v", err)
	}
	if cnt < 2 || reclaimed <= 0 || len(reaped) < 2 {
		t.Fatalf("expected at least 2 truncated logs, got %d (reclaimed %d)", cnt, reclaimed)
	}

	medData, err := fileutil.ReadFile(mediumLog)
	if err != nil {
		t.Fatalf("failed to read mediumLog: %v", err)
	}
	medParts := strings.Split(string(medData), "\n")
	if len(medParts) != 150 {
		t.Errorf("expected 150 lines retained in medium log, got %d", len(medParts))
	}

	tinyData, err := fileutil.ReadFile(tinyLog)
	if err != nil {
		t.Fatalf("failed to read tinyLog: %v", err)
	}
	tinyParts := strings.Split(string(tinyData), "\n")
	if len(tinyParts) != 1 {
		t.Errorf("expected 1 line retained in tiny log, got %d", len(tinyParts))
	}
}

func TestCycle201_IsLogFile_Comprehensive(t *testing.T) {
	t.Parallel()
	negativeCases := []struct {
		path string
		name string
	}{
		{"/root/.zqk/scheduler", "issues.json"},
		{"/root/.zqk/scheduler", "metrics-summary.json"},
		{"/root/.zqk/daemon", "daemon.pid"},
		{"/root/.zqk/daemon", "heartbeat.keepalive"},
		{"/root/.zqk/data", "binary.dat"},
		{"/root/.zqk/config", "settings.yaml"},
	}
	for _, tc := range negativeCases {
		if isLogFile(tc.path, tc.name) {
			t.Errorf("expected isLogFile(%q, %q) to be false", tc.path, tc.name)
		}
	}

	positiveCases := []struct {
		path string
		name string
	}{
		{"/root/logs", "app.log"},
		{"/root/logs", "events.jsonl"},
		{"/root/logs", "out.stdout"},
		{"/root/logs", "err.stderr"},
		{"/root/logs", "io.stdio"},
		{"/root/logs", "run.out"},
		{"/root/logs", "run.err"},
		{"/root/.zqk/logs", "audit.json"},
		{"/root/.zqk/diagnostics", "dump.txt"},
		{"/root/.zqk/events", "stream.json"},
		{"/root/.zqk/reports", "summary.txt"},
		{"/root/custom", "daemon.stdout"},
		{"/root/custom", "daemon.stderr"},
		{"/root/custom", "daemon.stdio"},
	}
	for _, tc := range positiveCases {
		if !isLogFile(tc.path, tc.name) {
			t.Errorf("expected isLogFile(%q, %q) to be true", tc.path, tc.name)
		}
	}
}

func TestCycle201_ExecuteHygiene_AllFlagsAndErrors(t *testing.T) {
	tmpDir := t.TempDir()
	cleanRoot := filepath.Clean(tmpDir)

	oldTime := time.Now().Add(-48 * time.Hour)

	tempFile := filepath.Join(cleanRoot, paths.ProjectDataDir, paths.CacheDir, ".tmp-exec-1")
	createTestFile(t, tempFile, []byte("temp content"))
	setModTime(t, tempFile, oldTime)

	staleLock := filepath.Join(cleanRoot, paths.ProjectDataDir, "locks", "exec.lock")
	createTestFile(t, staleLock, []byte(""))
	setModTime(t, staleLock, oldTime)

	agedLog := filepath.Join(cleanRoot, paths.ProjectDataDir, paths.LogsDir, "exec.log")
	createTestFile(t, agedLog, []byte("log entry\n"))
	setModTime(t, agedLog, oldTime)

	orphanSC := filepath.Join(cleanRoot, paths.ProjectDataDir, "scorecards", "missing__target.json")
	createTestFile(t, orphanSC, []byte(`{"file_path": "pkg/missing.go"}`))

	agedMetric := filepath.Join(cleanRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsCommandMetricsSubdir, "metric.chunk")
	createTestFile(t, agedMetric, []byte("metric payload"))
	setModTime(t, agedMetric, oldTime)

	opts := HygieneOptions{
		ReapLocks:        true,
		ReapTemp:         true,
		ReapProcesses:    true,
		EnforceRetention: true,
		ReapScorecards:   true,
		ReapMetrics:      true,
		DryRun:           true,
		LockThreshold:    1 * time.Hour,
		TempThreshold:    1 * time.Hour,
		LogMaxAge:        24 * time.Hour,
		LogMaxSize:       10 * 1024 * 1024,
		MetricsMaxAge:    24 * time.Hour,
	}

	reportDry, err := ExecuteHygiene(cleanRoot, opts)
	if err != nil {
		t.Fatalf("dry run ExecuteHygiene failed: %v", err)
	}
	if !reportDry.DryRun {
		t.Errorf("expected reportDry.DryRun=true")
	}
	if reportDry.TempReaped < 1 || reportDry.LocksReaped < 1 || reportDry.LogsPruned < 1 ||
		reportDry.ScorecardsReaped < 1 || reportDry.MetricsReaped < 1 {
		t.Errorf("dry run counts missing: %+v", reportDry)
	}

	opts.DryRun = false
	reportLive, err := ExecuteHygiene(cleanRoot, opts)
	if err != nil {
		t.Fatalf("live run ExecuteHygiene failed: %v", err)
	}
	if reportLive.DryRun {
		t.Errorf("expected reportLive.DryRun=false")
	}

	origList := listProcessesHook
	defer func() { listProcessesHook = origList }()
	listProcessesHook = func() ([]procEntry, error) {
		return nil, errors.New("injected process listing error")
	}
	reportWithErr, err := ExecuteHygiene(cleanRoot, opts)
	if err != nil {
		t.Fatalf("ExecuteHygiene returned unexpected error: %v", err)
	}
	if len(reportWithErr.Errors) == 0 {
		t.Errorf("expected error in report.Errors when listProcessesHook fails")
	}
}

func TestCycle201_InspectIOResources_SpecialDirsAndOrphans(t *testing.T) {
	tmpDir := t.TempDir()
	cleanRoot := filepath.Clean(tmpDir)

	for _, special := range []string{"cas", "blobs", "wal", ".git"} {
		dir := filepath.Join(cleanRoot, paths.ProjectDataDir, special)
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to mkdir special dir %s: %v", special, err)
		}
		dummy := filepath.Join(dir, "ignored.lock")
		createTestFile(t, dummy, []byte(""))
		setModTime(t, dummy, time.Now().Add(-2*time.Hour))
	}

	daemonLock := filepath.Join(cleanRoot, paths.ProjectDataDir, paths.StateDir, "daemon_locks", "steward.lock")
	createTestFile(t, daemonLock, []byte(""))
	setModTime(t, daemonLock, time.Now().Add(-2*time.Hour))

	staleLock := filepath.Join(cleanRoot, paths.ProjectDataDir, "locks", "dead.lock")
	createTestFile(t, staleLock, []byte(""))
	setModTime(t, staleLock, time.Now().Add(-2*time.Hour))

	orphanedTemp := filepath.Join(cleanRoot, paths.ProjectDataDir, paths.CacheDir, ".tmp-orphaned-subsystem")
	createTestFile(t, orphanedTemp, []byte("temporary data"))
	setModTime(t, orphanedTemp, time.Now().Add(-2*time.Hour))

	origList := listProcessesHook
	origCmd := getCommandLineHook
	defer func() {
		listProcessesHook = origList
		getCommandLineHook = origCmd
	}()

	listProcessesHook = func() ([]procEntry, error) {
		return []procEntry{
			{pid: 8888, ppid: 1, exe: "zqk"},
		}, nil
	}
	getCommandLineHook = func(pid int) (string, error) {
		if pid == 8888 {
			return "zqk orchestrate " + cleanRoot, nil
		}
		return "", nil
	}

	telemetry, err := InspectIOResources(context.Background(), cleanRoot)
	if err != nil {
		t.Fatalf("InspectIOResources failed: %v", err)
	}
	if telemetry.StaleLocksCount != 1 {
		t.Errorf("expected 1 stale lock, got %d (%v)", telemetry.StaleLocksCount, telemetry.StaleLockPaths)
	}
	if telemetry.OrphanedTempCount != 1 {
		t.Errorf("expected 1 orphaned temp, got %d (%v)", telemetry.OrphanedTempCount, telemetry.OrphanedTempPaths)
	}
	if telemetry.OrphanedProcessCount != 1 || len(telemetry.OrphanedProcesses) != 1 {
		t.Errorf("expected 1 orphaned process reported, got count=%d, list=%v", telemetry.OrphanedProcessCount, telemetry.OrphanedProcesses)
	}
}

func TestCycle201_FDUsage_Platform(t *testing.T) {
	fds, limit, err := GetProcessFDUsage()
	if runtime.GOOS == "windows" {
		if fds != -1 || limit != -1 || err != nil {
			t.Errorf("expected -1, -1, nil on windows, got %d, %d, %v", fds, limit, err)
		}
	} else {
		if err != nil {
			t.Logf("GetProcessFDUsage returned err on non-windows platform: %v", err)
		} else {
			if fds < 0 {
				t.Errorf("expected positive open FD count, got %d", fds)
			}
			if limit <= 0 {
				t.Logf("NOFILE limit is %d", limit)
			}
		}
	}

	tempNames := []string{
		"file.tmp",
		"data.tmp-1234",
		"test.tmp.bak",
		".tmp-hidden",
		"config.yaml.tmp",
		"pclimits-run-1",
	}
	for _, name := range tempNames {
		if !IsTempFileName(name) {
			t.Errorf("expected %q to be recognized as temporary", name)
		}
	}
	if IsTempFileName("regular.txt") || IsTempFileName("config.yaml") {
		t.Errorf("regular files should not be recognized as temporary")
	}

	if !IsLockFileName("service.lock") || IsLockFileName("service.locked") {
		t.Errorf("lock file recognition failed")
	}
}

func TestCycle201_ConcurrentHygieneOperations(t *testing.T) {
	tmpDir := t.TempDir()
	cleanRoot := filepath.Clean(tmpDir)

	for i := 0; i < 10; i++ {
		tmpPath := filepath.Join(cleanRoot, paths.ProjectDataDir, paths.CacheDir, fmt.Sprintf(".tmp-concurrent-%d", i))
		createTestFile(t, tmpPath, []byte("data"))
		setModTime(t, tmpPath, time.Now().Add(-2*time.Hour))
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	concurrency := 4

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		builder := goroutinelabels.NewGoroutine("resourcehygiene_worker", "concurrent sweep execution")
		builder.StartSimple(func() {
			defer wg.Done()
			if _, err := InspectIOResources(ctx, cleanRoot); err != nil {
				t.Errorf("concurrent inspect failed: %v", err)
			}
			if _, _, _, err := ReapOrphanedTempFiles(cleanRoot, 1*time.Hour, true); err != nil {
				t.Errorf("concurrent reap temp failed: %v", err)
			}
			if _, _, err := ReapStaleLocks(cleanRoot, 1*time.Hour, true); err != nil {
				t.Errorf("concurrent reap locks failed: %v", err)
			}
		})
	}

	wg.Wait()
}

func TestCycle201_StaleLocks_UnreadableAndFresh(t *testing.T) {
	tmpDir := t.TempDir()
	cleanRoot := filepath.Clean(tmpDir)

	freshLock := filepath.Join(cleanRoot, paths.ProjectDataDir, "locks", "fresh.lock")
	createTestFile(t, freshLock, []byte(""))
	setModTime(t, freshLock, time.Now())

	cnt, reaped, err := ReapStaleLocks(cleanRoot, 15*time.Minute, false)
	if err != nil {
		t.Fatalf("ReapStaleLocks failed: %v", err)
	}
	if cnt != 0 || len(reaped) != 0 {
		t.Errorf("expected fresh lock not to be reaped, got %d (%v)", cnt, reaped)
	}

	heldLock := filepath.Join(cleanRoot, paths.ProjectDataDir, "locks", "actively_held.lock")
	createTestFile(t, heldLock, []byte(""))
	setModTime(t, heldLock, time.Now().Add(-2*time.Hour))

	unreadableLock := filepath.Join(cleanRoot, paths.ProjectDataDir, "locks", "unreadable.lock")
	createTestFile(t, unreadableLock, []byte(""))
	setModTime(t, unreadableLock, time.Now().Add(-2*time.Hour))
	if err := fileutil.Chmod(unreadableLock, 0000); err == nil {
		defer func() {
			if errRest := fileutil.Chmod(unreadableLock, paths.FilePerm600); errRest != nil {
				t.Logf("failed to restore perm: %v", errRest)
			}
		}()
	}

	f, errOpen := fileutil.OpenFile(heldLock, fileutil.O_RDWR, paths.FilePerm600)
	if errOpen != nil {
		t.Fatalf("failed to open held lock: %v", errOpen)
	}
	defer f.Close()
	if err := syscallutil.FileFlock(f, syscall.LOCK_EX); err != nil {
		t.Fatalf("failed to flock: %v", err)
	}

	cntHeld, _, err := ReapStaleLocks(cleanRoot, 15*time.Minute, false)
	if err != nil {
		t.Fatalf("ReapStaleLocks on held lock failed: %v", err)
	}
	if cntHeld != 0 {
		t.Errorf("actively held lock must not be reaped")
	}
}
