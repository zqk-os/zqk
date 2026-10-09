package asynccheck_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck/asynccheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestCoherence_Comprehensive(t *testing.T) {
	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	filePath := filepath.Join(tmpDir, "GOAL-001.yaml")
	require.NoError(t, fileutil.WriteFile(filePath, []byte("id: GOAL-001\nkind: goal\n"), paths.FilePerm600))

	// 1. InferKindFromID
	kind := asynccheck.InferKindFromID("GOAL-001")
	require.NotEmpty(t, kind)

	// 2. ShouldUseCachedState with nil state
	require.False(t, asynccheck.ShouldUseCachedState("GOAL-001", filePath, nil, false))

	// 3. Valid clean state
	validState := &validation.ValidationState{
		ObjectID:      "GOAL-001",
		ObjectKind:    kind,
		FilePath:      filePath,
		LastValidated: time.Now().Add(1 * time.Hour),
		Issues:        nil,
	}
	require.True(t, asynccheck.ShouldUseCachedState("GOAL-001", filePath, validState, false))

	// 4. bypassCacheWithIssues
	stateWithWarning := &validation.ValidationState{
		ObjectID:      "GOAL-001",
		ObjectKind:    kind,
		FilePath:      filePath,
		LastValidated: time.Now().Add(1 * time.Hour),
		Issues:        []validation.ValidationIssue{{Category: "style", Tier: 2}},
	}
	require.True(t, asynccheck.ShouldUseCachedState("GOAL-001", filePath, stateWithWarning, false))
	require.False(t, asynccheck.ShouldUseCachedState("GOAL-001", filePath, stateWithWarning, true))

	// 5. Inconsistent kind
	inconsistentKindState := &validation.ValidationState{
		ObjectID:      "GOAL-001",
		ObjectKind:    "unknown_mismatch_kind",
		FilePath:      filePath,
		LastValidated: time.Now().Add(1 * time.Hour),
	}
	require.False(t, asynccheck.ShouldUseCachedState("GOAL-001", filePath, inconsistentKindState, false))

	// 6. Inconsistent path
	inconsistentPathState := &validation.ValidationState{
		ObjectID:      "GOAL-001",
		ObjectKind:    kind,
		FilePath:      filepath.Join(tmpDir, "other.yaml"),
		LastValidated: time.Now().Add(1 * time.Hour),
	}
	require.False(t, asynccheck.ShouldUseCachedState("GOAL-001", filePath, inconsistentPathState, false))

	// 7. Missing file on disk
	require.False(t, asynccheck.ShouldUseCachedState("GOAL-001", filepath.Join(tmpDir, "non_existent.yaml"), validState, false))

	// 8. Stale validation (file modified after last validation)
	staleState := &validation.ValidationState{
		ObjectID:      "GOAL-001",
		ObjectKind:    kind,
		FilePath:      filePath,
		LastValidated: time.Now().Add(-1 * time.Hour),
	}
	require.False(t, asynccheck.ShouldUseCachedState("GOAL-001", filePath, staleState, false))

	// 9. All invalidating issue branches
	invalidatingCases := []validation.ValidationIssue{
		{Category: validation.CategoryIntegrity},
		{Tier: 1, Category: validation.CategoryInstanceValidation},
		{Tier: 1, Category: validation.CategoryValidationError},
		{Tier: 1, Category: validation.CategoryValidationTimeout},
		{Tier: 1, Message: "validation timeout after 10s"},
		{Tier: 1, Category: asynccheck.CategoryGhostRef},
		{Tier: 1, Category: "GHOSTREF"},
		{Category: asynccheck.CategoryCacheLag},
		{Category: asynccheck.CategoryCacheCoherence},
	}

	for _, issue := range invalidatingCases {
		st := &validation.ValidationState{
			ObjectID:      "GOAL-001",
			ObjectKind:    kind,
			FilePath:      filePath,
			LastValidated: time.Now().Add(1 * time.Hour),
			Issues:        []validation.ValidationIssue{issue},
		}
		require.False(t, asynccheck.ShouldUseCachedState("GOAL-001", filePath, st, false), "issue %+v should invalidate cache", issue)
	}
}

func TestDiscovery_ExtractAndCASIndex(t *testing.T) {
	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	logger := logging.GetLoggerFromProfile("system")

	// 1. Legacy ExtractObjectIDFromFile wrapper
	f1 := filepath.Join(tmpDir, "BLI-100.yaml")
	require.NoError(t, fileutil.WriteFile(f1, []byte("id: BLI-100\nkind: backlog_item\n"), paths.FilePerm600))
	id := asynccheck.ExtractObjectIDFromFile(f1, "backlog_item")
	require.Equal(t, "BLI-100", id)

	// 2. CAS file name with content extraction
	casHash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	casFile := filepath.Join(tmpDir, casHash+".yaml")
	require.NoError(t, fileutil.WriteFile(casFile, []byte("id: CAS-OBJ-001\n"), paths.FilePerm600))
	idCAS := asynccheck.ExtractObjectIDFromFileWithContext(context.Background(), casFile, "criteria", logger)
	require.Equal(t, "CAS-OBJ-001", idCAS)

	// 3. Cancelled context
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()
	require.Empty(t, asynccheck.ExtractObjectIDFromFileWithContext(ctxCancel, casFile, "criteria", logger))

	// 4. File without id line (using 64-char hash so it attempts content parsing)
	noIDHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	noIDFile := filepath.Join(tmpDir, noIDHash+".yaml")
	require.NoError(t, fileutil.WriteFile(noIDFile, []byte("description: no id here\n"), paths.FilePerm600))
	require.Empty(t, asynccheck.ExtractObjectIDFromFileWithContext(context.Background(), noIDFile, "criteria", logger))

	// 5. Empty file (using 64-char hash so it attempts content parsing)
	emptyHash := "0000000000000000000000000000000000000000000000000000000000000000"
	emptyFile := filepath.Join(tmpDir, emptyHash+".yaml")
	require.NoError(t, fileutil.WriteFile(emptyFile, []byte(""), paths.FilePerm600))
	require.Empty(t, asynccheck.ExtractObjectIDFromFileWithContext(context.Background(), emptyFile, "criteria", logger))

	// 6. Non-existent file
	missingHash := "1111111111111111111111111111111111111111111111111111111111111111"
	missingFile := filepath.Join(tmpDir, missingHash+".yaml")
	require.Empty(t, asynccheck.ExtractObjectIDFromFileWithContext(context.Background(), missingFile, "criteria", logger))

	// 7. CAS index scan with date bucket directory
	criteriaDir := filepath.Join(tmpDir, "criteria")
	require.NoError(t, fileutil.MkdirAll(criteriaDir, paths.DirPerm750))
	cas := storage.OpenContentAddressableStorage(criteriaDir, "criteria")
	require.NoError(t, cas.Create("CRIT-001", []byte("id: CRIT-001\nkind: criteria\n")))

	// Date bucket directory
	bucketDir := filepath.Join(criteriaDir, "2026-10-08")
	require.NoError(t, fileutil.MkdirAll(bucketDir, paths.DirPerm750))

	sp, err := storage.NewFileObjectStorage(tmpDir)
	require.NoError(t, err)

	files, err := asynccheck.ScanObjectFilesWithContext(context.Background(), criteriaDir, "criteria", logger, sp)
	require.NoError(t, err)
	require.NotEmpty(t, files)
	require.Equal(t, "CRIT-001", files[0].ObjectID)

	// CAS index present but empty
	emptyKindDir := filepath.Join(tmpDir, "empty_kind")
	require.NoError(t, fileutil.MkdirAll(emptyKindDir, paths.DirPerm750))
	require.NoError(t, fileutil.WriteFile(filepath.Join(emptyKindDir, ".empty_kind.index"), []byte("{}"), paths.FilePerm600))
	filesEmpty, err := asynccheck.ScanObjectFilesWithContext(context.Background(), emptyKindDir, "empty_kind", logger, sp)
	require.NoError(t, err)
	require.Empty(t, filesEmpty)
}

func TestProgress_CoordinationAndFormatting(t *testing.T) {
	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { storage.ScrubProjectRootForTempCleanup(tmpDir, 50, 25*time.Millisecond) })
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	require.NoError(t, fileutil.MkdirAll(processDir, paths.DirPerm750))

	sp, err := storage.NewFileObjectStorage(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()

	// EmitCheckProgressEventViaCoordinator across different event types
	eventTypes := []string{
		asynccheck.ProgressEventTypeProgress,
		asynccheck.ProgressEventTypeSummary,
		asynccheck.ProgressEventTypeCompleted,
		asynccheck.ProgressEventTypeTimeout,
		asynccheck.ProgressEventTypeStuck,
		"custom_phase",
	}

	for _, et := range eventTypes {
		asynccheck.EmitCheckProgressEventViaCoordinator(
			ctx,
			tmpDir,
			sp,
			"op-123",
			et,
			50,
			100,
			45,
			5,
			10,
			"processing batch",
			"system",
		)
	}

	// Nil storage provider and edge condition (progress > totalTasks, totalTasks <= 0)
	asynccheck.EmitCheckProgressEventViaCoordinator(
		ctx,
		tmpDir,
		nil,
		"op-456",
		asynccheck.ProgressEventTypeCompleted,
		150,
		100,
		100,
		0,
		0,
		"",
		"system",
	)

	asynccheck.EmitCheckProgressEventViaCoordinator(
		ctx,
		tmpDir,
		nil,
		"op-789",
		asynccheck.ProgressEventTypeProgress,
		0,
		0,
		0,
		0,
		0,
		"",
		"system",
	)

	// Formatting tests
	summary := asynccheck.FormatProgressSummary(110.0, 110, 100, 0, 50.0, "scanning", 8, 16, 3700*time.Second)
	require.Contains(t, summary, "100.0%")
	require.Contains(t, summary, "scanning")
	require.Contains(t, summary, "01:01:40")

	summaryNoPhase := asynccheck.FormatProgressSummary(0.0, 0, 100, 10, 0.0, "", 0, 0, 45*time.Second)
	require.Contains(t, summaryNoPhase, "0.0%")
	require.NotContains(t, summaryNoPhase, "workers")

	// Duration formatting
	require.Equal(t, "01:02:03", asynccheck.FormatDuration(3723*time.Second))
	require.Equal(t, "05:30", asynccheck.FormatDuration(330*time.Second))
	require.Equal(t, "15s", asynccheck.FormatDuration(15*time.Second))
}

func TestRouterCoordination_Full(t *testing.T) {
	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { storage.ScrubProjectRootForTempCleanup(tmpDir, 50, 25*time.Millisecond) })
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	require.NoError(t, fileutil.MkdirAll(processDir, paths.DirPerm750))

	sp, err := storage.NewFileObjectStorage(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()

	// EmitAsyncRouterEventViaCoordinator variations
	// 1. High severity (status == "error")
	asynccheck.EmitAsyncRouterEventViaCoordinator(
		ctx,
		tmpDir,
		sp,
		"worker-err",
		"worker_failure",
		"error",
		4, 10, 8,
		5*time.Second,
	)

	// 2. High severity (failedCount > processedCount/2)
	asynccheck.EmitAsyncRouterEventViaCoordinator(
		ctx,
		tmpDir,
		sp,
		"worker-fail",
		"worker_batch",
		"degraded",
		4, 10, 6,
		0,
	)

	// 3. Medium severity (failedCount > 0)
	asynccheck.EmitAsyncRouterEventViaCoordinator(
		ctx,
		tmpDir,
		"not-a-storage-provider",
		"worker-med",
		"worker_batch",
		"success",
		4, 10, 1,
		2*time.Second,
	)

	// 4. Low severity (failedCount == 0)
	asynccheck.EmitAsyncRouterEventViaCoordinator(
		ctx,
		tmpDir,
		nil,
		"worker-clean",
		"worker_batch",
		"success",
		4, 10, 0,
		time.Second,
	)

	// EmitAsyncValidatorEventViaCoordinator variations
	// 1. eventType == "metrics"
	asynccheck.EmitAsyncValidatorEventViaCoordinator(
		ctx,
		tmpDir,
		sp,
		"op-metrics",
		"metrics",
		"OBJ-1",
		"metric message",
		map[string]any{"rate": 100},
		asynccheck.SeverityLow,
		"system",
	)

	// 2. severity == SeverityLow
	asynccheck.EmitAsyncValidatorEventViaCoordinator(
		ctx,
		tmpDir,
		sp,
		"op-low",
		"validation_pass",
		"OBJ-2",
		"",
		nil,
		asynccheck.SeverityLow,
		"system",
	)

	// 3. severity == SeverityHigh
	asynccheck.EmitAsyncValidatorEventViaCoordinator(
		ctx,
		tmpDir,
		sp,
		"op-high",
		"validation_failure",
		"OBJ-3",
		"schema error",
		map[string]any{"err_code": 400},
		asynccheck.SeverityHigh,
		"system",
	)

	// 4. severity == SeverityCritical
	asynccheck.EmitAsyncValidatorEventViaCoordinator(
		ctx,
		tmpDir,
		sp,
		"op-crit",
		"validation_panic",
		"",
		"critical failure",
		nil,
		asynccheck.SeverityCritical,
		"system",
	)

	// 5. severity == SeverityMedium without message or objectID
	asynccheck.EmitAsyncValidatorEventViaCoordinator(
		ctx,
		tmpDir,
		sp,
		"op-med",
		"validation_warn",
		"",
		"",
		map[string]any{"warning": "slow"},
		asynccheck.SeverityMedium,
		"system",
	)

	// 6. severity == "" (default)
	asynccheck.EmitAsyncValidatorEventViaCoordinator(
		ctx,
		tmpDir,
		nil,
		"op-def",
		"validation_default",
		"OBJ-4",
		"default sev",
		nil,
		"",
		"system",
	)
}
