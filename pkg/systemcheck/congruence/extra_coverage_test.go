package congruence_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objectidcache"
	"github.com/zqk-os/zqk/pkg/operational"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck/congruence"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestGatherCacheStatus_Loaded(t *testing.T) {
	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	goalsDir := filepath.Join(processDir, "goals")
	require.NoError(t, fileutil.MkdirAll(goalsDir, paths.DirPerm750))
	cacheDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir)
	require.NoError(t, fileutil.MkdirAll(cacheDir, paths.DirPerm750))

	objCache := objectidcache.GetGlobalObjectIDCache()
	objCache.ClearInMemoryCache(tmpDir)
	goalFile := filepath.Join(goalsDir, "GOAL-TEST-001.yaml")
	require.NoError(t, fileutil.WriteFile(goalFile, []byte("id: GOAL-TEST-001\nkind: goal\n"), paths.FilePerm600))
	objCache.Set("GOAL-TEST-001", &objectidcache.ObjectIDCacheEntry{
		ID:       "GOAL-TEST-001",
		Kind:     "goal",
		FilePath: goalFile,
		MTime:    time.Now(),
		Exists:   true,
	})
	require.NoError(t, objCache.SaveCache(tmpDir))

	revIndex := storage.GetGlobalReverseReferenceIndex()
	revIndex.AddReference("GOAL-TEST-001", "TASK-TEST-001")
	require.NoError(t, revIndex.SaveCache(tmpDir))

	status := congruence.GatherCacheStatus(tmpDir)
	require.True(t, status.ObjectIDCache.Loaded)
	require.GreaterOrEqual(t, status.ObjectIDCache.Total, 1)
}

func TestGatherProcessIntegrity_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	require.NoError(t, fileutil.MkdirAll(processDir, paths.DirPerm750))

	// 1. Valid goal file
	goalsDir := filepath.Join(processDir, "goals")
	require.NoError(t, fileutil.MkdirAll(goalsDir, paths.DirPerm750))
	validGoalFile := filepath.Join(goalsDir, "GOAL-001.yaml")
	require.NoError(t, fileutil.WriteFile(validGoalFile, []byte("id: GOAL-001\nkind: goal\n"), paths.FilePerm600))

	// 2. Unmanaged file (in goals directory but not in cache)
	unmanagedGoalFile := filepath.Join(goalsDir, "GOAL-UNMANAGED.yaml")
	require.NoError(t, fileutil.WriteFile(unmanagedGoalFile, []byte("id: GOAL-UNMANAGED\nkind: goal\n"), paths.FilePerm600))

	// 3. Ignored files: non-yaml, dotfile, missing id
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalsDir, "notes.txt"), []byte("text"), paths.FilePerm600))
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalsDir, ".hidden.yaml"), []byte("id: H\n"), paths.FilePerm600))
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalsDir, "noid.yaml"), []byte("foo: bar\n"), paths.FilePerm600))

	// 4. Misplaced file: kind is "goal" but stored in "tasks"
	tasksDir := filepath.Join(processDir, "tasks")
	require.NoError(t, fileutil.MkdirAll(tasksDir, paths.DirPerm750))
	misplacedFile := filepath.Join(tasksDir, "MISPLACED-GOAL.yaml")
	require.NoError(t, fileutil.WriteFile(misplacedFile, []byte("id: GOAL-002\nkind: goal\n"), paths.FilePerm600))

	// 5. Stream-backed dir (should be skipped by walk)
	metricsDir := filepath.Join(processDir, "metrics")
	require.NoError(t, fileutil.MkdirAll(metricsDir, paths.DirPerm750))
	require.NoError(t, fileutil.WriteFile(filepath.Join(metricsDir, "metric.yaml"), []byte("id: MET-1\nkind: metrics\n"), paths.FilePerm600))

	// 6. _internal dir (should be skipped)
	internalDir := filepath.Join(processDir, congruence.OCRProcessInternalDir)
	require.NoError(t, fileutil.MkdirAll(internalDir, paths.DirPerm750))
	require.NoError(t, fileutil.WriteFile(filepath.Join(internalDir, "internal.yaml"), []byte("id: INT-1\nkind: internal\n"), paths.FilePerm600))

	// 7. Unknown dir (not mapped to any kind)
	unknownDir := filepath.Join(processDir, "some_unknown_directory")
	require.NoError(t, fileutil.MkdirAll(unknownDir, paths.DirPerm750))

	// 8. Hidden dir at process root (skipped)
	hiddenDir := filepath.Join(processDir, ".cache_tmp")
	require.NoError(t, fileutil.MkdirAll(hiddenDir, paths.DirPerm750))

	// 9. Stray file at process root
	require.NoError(t, fileutil.WriteFile(filepath.Join(processDir, "stray.txt"), []byte("stray"), paths.FilePerm600))

	// Update object ID cache with GOAL-001 so it's managed, but leave GOAL-UNMANAGED out
	objCache := objectidcache.GetGlobalObjectIDCache()
	objCache.Set("GOAL-001", &objectidcache.ObjectIDCacheEntry{
		ID:       "GOAL-001",
		Kind:     "goal",
		FilePath: validGoalFile,
		Exists:   true,
	})
	require.NoError(t, objCache.SaveCache(tmpDir))

	integrity := congruence.GatherProcessIntegrity(tmpDir, []string{"metrics"})
	require.NotEmpty(t, integrity.UnmanagedFiles)
	require.Contains(t, integrity.UnknownDirs, "some_unknown_directory")
	require.Contains(t, integrity.FilesAtRoot, "stray.txt")
}

func TestGatherObjectCountByKindForReport(t *testing.T) {
	ctx := context.Background()

	// Empty root returns nil
	require.Nil(t, congruence.GatherObjectCountByKindForReport(ctx, "", nil, false))

	// noCache returns nil
	require.Nil(t, congruence.GatherObjectCountByKindForReport(ctx, "/some/root", nil, true))

	// Real project root with cache
	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	goalsDir := filepath.Join(processDir, "goals")
	reqsDir := filepath.Join(processDir, "requirements")
	require.NoError(t, fileutil.MkdirAll(goalsDir, paths.DirPerm750))
	require.NoError(t, fileutil.MkdirAll(reqsDir, paths.DirPerm750))
	cacheDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir)
	require.NoError(t, fileutil.MkdirAll(cacheDir, paths.DirPerm750))

	sp, err := storage.NewFileObjectStorage(tmpDir)
	require.NoError(t, err)

	goalFile := filepath.Join(goalsDir, "GOAL-1.yaml")
	reqFile := filepath.Join(reqsDir, "REQ-1.yaml")
	require.NoError(t, fileutil.WriteFile(goalFile, []byte("id: GOAL-1\nkind: goal\n"), paths.FilePerm600))
	require.NoError(t, fileutil.WriteFile(reqFile, []byte("id: REQ-1\nkind: requirement\n"), paths.FilePerm600))

	objCache := objectidcache.GetGlobalObjectIDCache()
	objCache.ClearInMemoryCache(tmpDir)
	objCache.Set("GOAL-1", &objectidcache.ObjectIDCacheEntry{
		ID:       "GOAL-1",
		Kind:     "goal",
		FilePath: goalFile,
		MTime:    time.Now(),
		Exists:   true,
	})
	objCache.Set("REQ-1", &objectidcache.ObjectIDCacheEntry{
		ID:       "REQ-1",
		Kind:     "requirement",
		FilePath: reqFile,
		MTime:    time.Now(),
		Exists:   true,
	})
	require.NoError(t, objCache.SaveCache(tmpDir))

	counts := congruence.GatherObjectCountByKindForReport(ctx, tmpDir, sp, false)
	require.NotNil(t, counts)
	require.Equal(t, 1, counts["goal"])
	require.Equal(t, 1, counts["requirement"])
}

func TestMetricsRecordingAndPruning(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("system")

	// 1. RecordObjectVolumeMetrics with nil report and empty root
	congruence.RecordObjectVolumeMetrics("", nil, logger, 10)
	congruence.RecordObjectVolumeMetrics(tmpDir, nil, logger, 10)

	// Valid report
	report := &operational.CongruenceReport{
		GeneratedAt:       time.Now().UTC(),
		ProjectRoot:       tmpDir,
		ObjectCountByKind: map[string]int{"goal": 15, "task": 20},
	}
	congruence.RecordObjectVolumeMetrics(tmpDir, report, logger, 10)

	// 2. RecordStreamVolumeMetrics with empty and valid root
	congruence.RecordStreamVolumeMetrics("", logger, 10)
	congruence.RecordStreamVolumeMetrics(tmpDir, logger, 10)

	// 3. RecordFilesystemSnapshotMetrics with nil and valid snapshot
	congruence.RecordFilesystemSnapshotMetrics("", nil, logger, 10)
	congruence.RecordFilesystemSnapshotMetrics(tmpDir, nil, logger, 10)

	snap := &operational.FilesystemProjectSnapshot{
		SnapshotScope: "all",
		GeneratedAt:   "invalid-date-format-fallback",
		TotalFiles:    1234,
		TotalBytes:    567890,
		Volume: &operational.FilesystemVolumeStats{
			Source:         "/dev/disk1",
			TotalBytes:     1000000,
			AvailableBytes: 500000,
			FreeBytes:      400000,
		},
		ZqkByChild: map[string]*operational.FilesystemBucketStats{
			"process": {Files: 10, Bytes: 200},
			"metrics": {Files: 5, Bytes: 100},
		},
	}
	congruence.RecordFilesystemSnapshotMetrics(tmpDir, snap, logger, 10)

	// Valid snapshot with valid RFC3339Nano date
	snap.GeneratedAt = time.Now().UTC().Format(time.RFC3339Nano)
	congruence.RecordFilesystemSnapshotMetrics(tmpDir, snap, logger, 10)

	// 4. PruneObjectVolumeChunks edge cases
	congruence.PruneObjectVolumeChunks(tmpDir, 0, logger)
	congruence.PruneObjectVolumeChunks(filepath.Join(tmpDir, "non_existent_dir"), 5, logger)

	// Create test files in a chunk dir
	chunkDir := filepath.Join(tmpDir, "chunks")
	require.NoError(t, fileutil.MkdirAll(chunkDir, paths.DirPerm750))

	// Old chunk file (should be pruned)
	oldChunk := filepath.Join(chunkDir, "old.chunk")
	require.NoError(t, fileutil.WriteFile(oldChunk, []byte("data"), paths.FilePerm600))
	oldTime := time.Now().Add(-100 * 24 * time.Hour)
	_ = os.Chtimes(oldChunk, oldTime, oldTime)

	// New chunk file (should remain)
	newChunk := filepath.Join(chunkDir, "new.chunk")
	require.NoError(t, fileutil.WriteFile(newChunk, []byte("data"), paths.FilePerm600))

	// Non-chunk file (should remain)
	nonChunk := filepath.Join(chunkDir, "readme.txt")
	require.NoError(t, fileutil.WriteFile(nonChunk, []byte("readme"), paths.FilePerm600))

	// Subdirectory (should remain)
	subDir := filepath.Join(chunkDir, "subdir")
	require.NoError(t, fileutil.MkdirAll(subDir, paths.DirPerm750))

	congruence.PruneObjectVolumeChunks(chunkDir, 30, logger)

	_, err := os.Stat(oldChunk)
	require.True(t, os.IsNotExist(err), "old chunk should be deleted")
	_, err = os.Stat(newChunk)
	require.NoError(t, err, "new chunk should remain")
}

func TestReportWriting_ExtensiveBranches(t *testing.T) {
	tmpDir := t.TempDir()
	reportDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir, paths.LogsReportsSubdir)
	require.NoError(t, fileutil.MkdirAll(reportDir, paths.DirPerm750))

	report := &operational.CongruenceReport{
		GeneratedAt:   time.Now().UTC(),
		ProjectRoot:   tmpDir,
		TotalDisk:     60000,
		TotalObject:   65000,
		TotalInternal: 64000,
		StrayDirs:     []string{"stray_dir_1", "stray_dir_2"},
		DiskCountByDir: map[string]int{
			"goals":        1000,
			"audit":        5500,
			"mcp_sessions": 600,
		},
		ObjectCountByKind: map[string]int{
			"goal":        1000,
			"audit_event": 60000,
		},
		InternalCountByKind: map[string]int{
			"goal": 1000,
		},
		DisparityByDir: map[string]int{
			"goals":        0,
			"audit":        5500,
			"mcp_sessions": 550,
		},
		KindsWithDisparity: []string{"audit", "mcp_sessions"},
		StreamBackedDirs:   []string{"audit", "metrics"},
		LegacyStreamBackedByDir: map[string]int{
			"audit": 150,
		},
		Alerts: []string{"alert 1: high disparity in audit"},
	}

	var cacheStatus congruence.CacheStatus
	cacheStatus.ObjectIDCache.Path = "/path/cache.bin"
	cacheStatus.ObjectIDCache.Loaded = true
	cacheStatus.ObjectIDCache.Total = 100
	cacheStatus.ObjectIDCache.CountByKind = map[string]int{"goal": 100}
	cacheStatus.ReverseReferenceIndex.Path = "/path/rev.bin"
	cacheStatus.ReverseReferenceIndex.Loaded = true
	cacheStatus.ReverseReferenceIndex.ReferencedIDCount = 50

	integrity := congruence.ProcessIntegrity{
		MisplacedByKind: map[string][]string{
			"goal": {"/path/tasks/GOAL-1.yaml"},
		},
		UnmanagedFiles: []string{"/path/goals/UNMANAGED.yaml"},
		UnknownDirs:    []string{"unknown_dir"},
		FilesAtRoot:    []string{"root_file.txt"},
	}

	// Create a snapshot with >25 top level buckets to exercise truncation branch
	byTopLevel := make(map[string]*operational.FilesystemBucketStats)
	for i := 0; i < 30; i++ {
		byTopLevel[fmt.Sprintf("bucket_%02d", i)] = &operational.FilesystemBucketStats{
			Files: int64(100 - i),
			Bytes: int64(1000 * (100 - i)),
		}
	}
	fsSnap := &operational.FilesystemProjectSnapshot{
		SnapshotScope: "all",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		TotalFiles:    10000,
		TotalBytes:    50000000,
		Volume: &operational.FilesystemVolumeStats{
			Source:         "/dev/sda1",
			TotalBytes:     1000000000,
			AvailableBytes: 500000000,
			FreeBytes:      400000000,
		},
		ByTopLevel: byTopLevel,
		ZqkByChild: map[string]*operational.FilesystemBucketStats{
			"process": {Files: 100, Bytes: 5000},
			"logs":    {Files: 50, Bytes: 2000},
		},
	}

	txtPath := filepath.Join(reportDir, "full-report.txt")
	require.NoError(t, congruence.WriteReportFile(txtPath, report, cacheStatus, integrity, fsSnap))

	data, err := fileutil.ReadFile(txtPath)
	require.NoError(t, err)
	content := string(data)
	require.Contains(t, content, "HIGH TOTAL")
	require.Contains(t, content, "Large gap (total_object - total_disk")
	require.Contains(t, content, "audit/ disparity=")
	require.Contains(t, content, "mcp_sessions/ disparity=")
	require.Contains(t, content, "Stray process dirs")
	require.Contains(t, content, "Misplaced files")
	require.Contains(t, content, "Unmanaged files")
	require.Contains(t, content, "Unknown dirs and trash")
	require.Contains(t, content, "more buckets")

	// Also test gap < -1000 interpretation branch
	reportNegativeGap := &operational.CongruenceReport{
		GeneratedAt:    time.Now().UTC(),
		ProjectRoot:    tmpDir,
		TotalDisk:      5000,
		TotalObject:    100,
		DisparityByDir: map[string]int{},
	}
	txtPathNeg := filepath.Join(reportDir, "neg-report.txt")
	require.NoError(t, congruence.WriteReportFile(txtPathNeg, reportNegativeGap, congruence.CacheStatus{}, congruence.ProcessIntegrity{}, nil))
	dataNeg, err := fileutil.ReadFile(txtPathNeg)
	require.NoError(t, err)
	require.Contains(t, string(dataNeg), "Large gap (total_disk - total_object")

	// Test UpdateReportsIndex with corrupt index file and with full index truncation
	logger := logging.GetLoggerFromProfile("system")
	indexPath := filepath.Join(reportDir, paths.ReportsIndexFile)
	require.NoError(t, fileutil.WriteFile(indexPath, []byte("{invalid json"), paths.FilePerm600))
	congruence.UpdateReportsIndex(reportDir, filepath.Join(reportDir, "report1.json"), report, logger)

	// Now add multiple entries to exceed ReportsIndexMaxEntries (100)
	for i := 0; i < 110; i++ {
		congruence.UpdateReportsIndex(reportDir, filepath.Join(reportDir, fmt.Sprintf("report_%d.json", i)), report, logger)
	}

	// Test WriteDashboardSnapshot with nil report and with snapshot
	congruence.WriteDashboardSnapshot(reportDir, nil, logger, nil)
	byTopLevel[paths.ProjectDataDir] = &operational.FilesystemBucketStats{Files: 500, Bytes: 25000}
	congruence.WriteDashboardSnapshot(reportDir, report, logger, fsSnap)
}

func TestResolveZQKBin(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Fallback when neither exists
	bin := congruence.ResolveZQKBin(tmpDir)
	require.Equal(t, "zqk-admin", bin)

	// 2. root/bin/zqk-admin exists
	binDir := filepath.Join(tmpDir, "bin")
	require.NoError(t, fileutil.MkdirAll(binDir, paths.DirPerm750))
	adminBin1 := filepath.Join(binDir, "zqk-admin")
	require.NoError(t, fileutil.WriteFile(adminBin1, []byte("#!/bin/sh\n"), paths.FilePerm755))
	require.Equal(t, adminBin1, congruence.ResolveZQKBin(tmpDir))

	// 3. root/zqk-admin exists when bin dir doesn't have it
	require.NoError(t, fileutil.Remove(adminBin1))
	adminBin2 := filepath.Join(tmpDir, "zqk-admin")
	require.NoError(t, fileutil.WriteFile(adminBin2, []byte("#!/bin/sh\n"), paths.FilePerm755))
	require.Equal(t, adminBin2, congruence.ResolveZQKBin(tmpDir))
}

func TestEmitCongruenceAlertEvent(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("system")
	report := &operational.CongruenceReport{
		GeneratedAt:        time.Now().UTC(),
		ProjectRoot:        tmpDir,
		TotalDisk:          10,
		TotalObject:        20,
		Alerts:             []string{"alert 1"},
		KindsWithDisparity: []string{"task"},
		DisparityByDir:     map[string]int{"tasks": 10},
	}
	congruence.EmitCongruenceAlertEvent(context.Background(), tmpDir, nil, report, logger)
}

func TestRunCongruence_FullPipeline(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "congruence-pipeline-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		time.Sleep(100 * time.Millisecond)
		_ = fileutil.RemoveAll(tmpDir)
	})
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	goalsDir := filepath.Join(processDir, "goals")
	require.NoError(t, fileutil.MkdirAll(goalsDir, paths.DirPerm750))
	goalFile := filepath.Join(goalsDir, "GOAL-001.yaml")
	require.NoError(t, fileutil.WriteFile(goalFile, []byte("id: GOAL-001\nkind: goal\n"), paths.FilePerm600))

	sp, err := storage.NewFileObjectStorage(tmpDir)
	require.NoError(t, err)

	// Pre-populate cache so TotalObject > TotalDisk, exercising stale cache cleanup branch
	objCache := objectidcache.GetGlobalObjectIDCache()
	objCache.Set("GOAL-001", &objectidcache.ObjectIDCacheEntry{
		ID:       "GOAL-001",
		Kind:     "goal",
		FilePath: goalFile,
		Exists:   true,
	})
	objCache.Set("GOAL-STALE-1", &objectidcache.ObjectIDCacheEntry{
		ID:       "GOAL-STALE-1",
		Kind:     "goal",
		FilePath: filepath.Join(goalsDir, "deleted1.yaml"),
		Exists:   true,
	})
	require.NoError(t, objCache.SaveCache(tmpDir))

	opts := congruence.Options{
		IncludeInternal:            false,
		IncludeFilesystemSnapshot:  true,
		FilesystemSnapshotScopeStr: "all",
		EmitEvents:                 true,
		UserProvidedReportFile:     true,
		OutputPath:                 filepath.Join(tmpDir, "custom-report.json"),
		NoCache:                    false,
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	result, err := congruence.RunCongruence(context.Background(), tmpDir, sp, secCtx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.TextReportPath)
	require.NotEmpty(t, result.JSONReportPath)
	require.FileExists(t, result.TextReportPath)
	require.FileExists(t, result.JSONReportPath)
}
