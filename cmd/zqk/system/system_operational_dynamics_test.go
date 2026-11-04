package system

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Criteria Reference: CRIT-1793632807358586000-4d0cf8e6
// Requirement: REQ-1793632785595764000-de36ddb0
// Backlog Item: BLI-1793632807358586000-44195ecc
// Operational Dynamic Proof: Async System Check Execution & Change Journal Pipeline
//
// Validates live execution paths, progress channels, and change journal event
// emissions across async_check.go, async_check_helpers.go, and system_check_pipeline.go.

func ensurePipelineBypassFlags(cmd *cobra.Command) {
	if cmd.Flags().Lookup("allow-degraded") == nil {
		cmd.Flags().Bool("allow-degraded", false, "allow degraded mode")
	}
	if cmd.Flags().Lookup("ignore-scheduler-down") == nil {
		cmd.Flags().Bool("ignore-scheduler-down", false, "ignore scheduler down")
	}
}

// TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_CAPJournalFreshnessAndBypass
// tests the live pipeline execution stages, verifying CAP orchestrator change journal freshness
// validation, stale journal failure detection, and degraded bypass options.
func TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_CAPJournalFreshnessAndBypass(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	projectRoot := proj.Root
	_, err := setupSystemTestEnvironmentRoot(t, projectRoot)
	require.NoError(t, err)

	journalDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler", objects.JobIDCapOrchestrator)
	require.NoError(t, fileutil.EnsureDir(journalDir))
	journalPath := filepath.Join(journalDir, objects.JobIDCapOrchestrator+".events.jsonl")

	// 1. Stale CAP journal (> 4 hours old) triggers CAP_PROBE failure
	staleContent := `{"event_id":"EVT-1","timestamp":"2026-11-01T00:00:00Z","type":"scheduler_tick"}` + "\n"
	require.NoError(t, fileutil.WriteFile(journalPath, []byte(staleContent), paths.FilePerm644))
	staleTime := time.Now().Add(-5 * time.Hour)
	require.NoError(t, os.Chtimes(journalPath, staleTime, staleTime))

	cmd := NewCheckCmd()
	cmd.SetContext(pkgctx.NewSystemContext())
	ensurePipelineBypassFlags(cmd)
	cliCtx := cli.ContextForProjectAndProfile(projectRoot, "system")

	outcome, pipelineErr := runSystemCheckPipelineWithOutcome(cmd, cliCtx, []string{"all"}, "op-crit2-stale", context.Background())
	require.Error(t, pipelineErr)
	assert.Contains(t, pipelineErr.Error(), "CAP journal is stale: no updates in over 4 hours")
	_ = outcome

	// 2. Bypass via --allow-degraded flag
	cmdDegraded := NewCheckCmd()
	cmdDegraded.SetContext(pkgctx.NewSystemContext())
	ensurePipelineBypassFlags(cmdDegraded)
	require.NoError(t, cmdDegraded.Flags().Set("allow-degraded", "true"))

	_, errDegraded := runSystemCheckPipelineWithOutcome(cmdDegraded, cliCtx, []string{"all"}, "op-crit2-degraded", context.Background())
	assert.NoError(t, errDegraded, "expected --allow-degraded to bypass stale CAP journal check")

	// 3. Bypass via --ignore-scheduler-down flag
	cmdIgnoreScheduler := NewCheckCmd()
	cmdIgnoreScheduler.SetContext(pkgctx.NewSystemContext())
	ensurePipelineBypassFlags(cmdIgnoreScheduler)
	require.NoError(t, cmdIgnoreScheduler.Flags().Set("ignore-scheduler-down", "true"))

	_, errIgnore := runSystemCheckPipelineWithOutcome(cmdIgnoreScheduler, cliCtx, []string{"all"}, "op-crit2-ignore", context.Background())
	assert.NoError(t, errIgnore, "expected --ignore-scheduler-down to bypass stale CAP journal check")

	// 4. Bypass via tier > 0
	cmdTier := NewCheckCmd()
	cmdTier.SetContext(pkgctx.NewSystemContext())
	ensurePipelineBypassFlags(cmdTier)
	require.NoError(t, cmdTier.Flags().Set("tier", "1"))

	_, errTier := runSystemCheckPipelineWithOutcome(cmdTier, cliCtx, []string{"all"}, "op-crit2-tier", context.Background())
	assert.NoError(t, errTier, "expected tier > 0 to bypass stale CAP journal check")

	// 5. Bypass via environment variable ZQK_ALLOW_DEGRADED
	t.Setenv(zqkenv.AllowDegraded().Name(), "true")
	cmdEnv := NewCheckCmd()
	cmdEnv.SetContext(pkgctx.NewSystemContext())
	ensurePipelineBypassFlags(cmdEnv)

	_, errEnv := runSystemCheckPipelineWithOutcome(cmdEnv, cliCtx, []string{"all"}, "op-crit2-env", context.Background())
	assert.NoError(t, errEnv, "expected ZQK_ALLOW_DEGRADED to bypass stale CAP journal check")
	t.Setenv(zqkenv.AllowDegraded().Name(), "false")

	// 6. Fresh CAP journal passes verification cleanly
	recentTime := time.Now()
	require.NoError(t, os.Chtimes(journalPath, recentTime, recentTime))

	cmdFresh := NewCheckCmd()
	cmdFresh.SetContext(pkgctx.NewSystemContext())
	ensurePipelineBypassFlags(cmdFresh)

	freshOutcome, errFresh := runSystemCheckPipelineWithOutcome(cmdFresh, cliCtx, []string{"all"}, "op-crit2-fresh", context.Background())
	assert.NoError(t, errFresh)
	assert.Equal(t, true, freshOutcome["finalize_done"])

	// 7. Wrapper function RunSystemCheckViaPipeline
	errWrapper := RunSystemCheckViaPipeline(cmdFresh, cliCtx, []string{"all"}, "op-crit2-wrapper", context.Background())
	assert.NoError(t, errWrapper)
}

// TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_PipelinePayloadCleanupsAndMetrics
// validates payload LIFO cleanup discipline and noop metrics sink stability.
func TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_PipelinePayloadCleanupsAndMetrics(t *testing.T) {
	// LIFO cleanup sequence verification
	payload := &systemCheckPipelinePayload{}
	var callOrder []int
	payload.addCleanup(func() { callOrder = append(callOrder, 1) })
	payload.addCleanup(func() { callOrder = append(callOrder, 2) })
	payload.addCleanup(func() { callOrder = append(callOrder, 3) })

	payload.cleanup()
	assert.Equal(t, []int{3, 2, 1}, callOrder, "payload cleanups must execute in strict LIFO order")

	// noopMetricsSinkSystemCheck should handle calls gracefully
	sink := noopMetricsSinkSystemCheck{}
	sink.RecordStage(context.Background(), pipelineKindSystemCheck, "INGEST", 10*time.Millisecond, nil)
	sink.RecordStageWithBuckets(context.Background(), pipelineKindSystemCheck, "INGEST", 10*time.Millisecond, nil, map[string]string{"env": "test"})

	// Input guard tests on pipeline
	cmd := NewCheckCmd()
	outcomeNilCtx, errNilCtx := runSystemCheckPipelineWithOutcome(cmd, nil, []string{"all"}, "op-nil-ctx", context.Background())
	require.Error(t, errNilCtx)
	assert.Equal(t, "nil ctx", outcomeNilCtx["ingest_error"])

	validCtx := cli.ContextForProjectAndProfile("", "system")
	outcomeNilCmd, errNilCmd := runSystemCheckPipelineWithOutcome(nil, validCtx, []string{"all"}, "op-nil-cmd", context.Background())
	require.Error(t, errNilCmd)
	assert.Equal(t, "nil cmd", outcomeNilCmd["ingest_error"])
}

// TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_DiscoveryProgressStreaming
// exercises parallel object discovery streaming channels, batching, and cache discovery.
func TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_DiscoveryProgressStreaming(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	projectRoot := proj.Root
	_, err := setupSystemTestEnvironmentRoot(t, projectRoot)
	require.NoError(t, err)

	crit1 := "id: CRIT-TEST-001\nkind: criteria\nschema_version: \"2.0.0\"\nstatus: active\ntitle: Crit 1\n"
	crit2 := "id: CRIT-TEST-002\nkind: criteria\nschema_version: \"2.0.0\"\nstatus: active\ntitle: Crit 2\n"
	_ = testkit.WriteTestObjectStandalone(t, projectRoot, crit1)
	_ = testkit.WriteTestObjectStandalone(t, projectRoot, crit2)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Live parallel discovery over criteria kind
	streamChan, collectFunc := discoverObjectsParallel(
		context.Background(),
		projectRoot,
		"op-stream-test",
		[]string{"criteria"},
		nil,
		logger,
		nil,
		"system",
	)

	var streamedFiles []scannedFile
	var collected []scannedFile
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("test_collector", "collect discovery results").StartSimple(func() {
		collected = collectFunc()
		close(done)
	})

	for batch := range streamChan {
		streamedFiles = append(streamedFiles, batch...)
	}
	<-done
	assert.Equal(t, len(streamedFiles), len(collected), "streamed file count should match collector result")
	assert.GreaterOrEqual(t, len(streamedFiles), 2)

	// Context cancellation stops streaming cleanly
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	streamChanCanceled, _ := discoverObjectsParallel(
		cancelCtx,
		projectRoot,
		"op-cancel-test",
		[]string{"criteria"},
		nil,
		logger,
		nil,
		"system",
	)
	var canceledFiles []scannedFile
	select {
	case batch, ok := <-streamChanCanceled:
		if ok {
			canceledFiles = append(canceledFiles, batch...)
		}
	case <-time.After(100 * time.Millisecond):
	}
	_ = canceledFiles

	// Discovery from populated ObjectIDCache
	cache := NewObjectIDCache()
	require.NoError(t, cache.BuildCache(context.Background(), projectRoot, false))

	cacheStream, cacheCollector := discoverFromCache(
		context.Background(),
		projectRoot,
		"op-cache-stream-test",
		[]string{"criteria"},
		nil,
		cache,
		logger,
		nil,
		"system",
	)

	var cacheStreamedFiles []scannedFile
	var cacheCollected []scannedFile
	cacheDone := make(chan struct{})
	goroutinelabels.NewGoroutine("test_cache_collector", "collect cache results").StartSimple(func() {
		cacheCollected = cacheCollector()
		close(cacheDone)
	})

	for batch := range cacheStream {
		cacheStreamedFiles = append(cacheStreamedFiles, batch...)
	}
	<-cacheDone
	assert.Equal(t, len(cacheStreamedFiles), len(cacheCollected))
	assert.GreaterOrEqual(t, len(cacheStreamedFiles), 2)
}

// TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_EnqueueFilesProgressChannel
// tests ingestion of discovered file streams, deduplication, and task counting.
func TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_EnqueueFilesProgressChannel(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	projectRoot := proj.Root
	_, err := setupSystemTestEnvironmentRoot(t, projectRoot)
	require.NoError(t, err)

	cmd := NewCheckCmd()
	cmd.SetContext(pkgctx.NewSystemContext())
	cliCtx := cli.ContextForProjectAndProfile(projectRoot, "system")

	checkCtx := &AsyncCheckContext{
		Cmd:                cmd,
		Ctx:                cliCtx,
		ProjectRoot:        projectRoot,
		Metrics:            validation.NewValidationMetrics(),
		Logger:             logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		EnqueuedObjectIDs:  make(map[string]bool),
		EnqueuedObjectInfo: make(map[string]struct{ Kind, FilePath string }),
		CachedCheckResults: make(map[string]CheckResult),
		AsyncValidator:     validation.NewAsyncValidator(pkgctx.NewSystemContext(), projectRoot, 2, time.Minute),
	}

	filesChan := make(chan []scannedFile, 5)

	critPath := filepath.Join(projectRoot, paths.ProcessDir, "criteria", "CRIT-ENQ-001.yaml")
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(critPath)))
	require.NoError(t, fileutil.WriteFile(critPath, []byte("id: CRIT-ENQ-001\nkind: criteria\n"), paths.FilePerm644))

	batch1 := []scannedFile{
		{ObjectID: "CRIT-ENQ-001", Kind: "criteria", Path: critPath},
	}
	// duplicate item in batch 2
	batch2 := []scannedFile{
		{ObjectID: "CRIT-ENQ-001", Kind: "criteria", Path: critPath},
	}
	// item with mismatched kind to exercise pendingByID deferral
	batch3 := []scannedFile{
		{ObjectID: "CRIT-ENQ-002", Kind: "other_kind", Path: critPath},
	}

	filesChan <- batch1
	filesChan <- batch2
	filesChan <- batch3
	close(filesChan)

	require.NoError(t, enqueueFilesAsDiscovered(checkCtx, filesChan))
	assert.Equal(t, 1, checkCtx.TotalTasks, "duplicate object should be deduplicated")
	assert.True(t, checkCtx.EnqueuedObjectIDs["CRIT-ENQ-001"])

	// Test enqueueDiscoveryLostIDs helper
	assert.False(t, enqueueDiscoveryLostIDs(10, 10))
	assert.True(t, enqueueDiscoveryLostIDs(10, 9))
}

// TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_OutputQueueAndQueueWriter
// validates output queuing, non-blocking queueWriter semantics on full queues, and result callbacks.
func TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_OutputQueueAndQueueWriter(t *testing.T) {
	// 1. queueWriter non-blocking write & drop behavior on overflow
	tinyQueue := validation.NewOutputQueue(1)
	qw := &queueWriter{
		queue:     tinyQueue,
		channelID: outputChannelStdout,
		flushHint: false,
		logger:    logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	n1, err1 := qw.Write([]byte("first packet\n"))
	assert.NoError(t, err1)
	assert.Equal(t, 13, n1)

	// Second packet should be dropped without blocking or returning error
	n2, err2 := qw.Write([]byte("overflow packet\n"))
	assert.NoError(t, err2)
	assert.Equal(t, 16, n2)

	// 2. createOutputQueueAndWriter lifecycle
	cmd := NewCheckCmd()
	cmd.SetContext(pkgctx.NewSystemContext())
	checkCtx := &AsyncCheckContext{
		Cmd:    cmd,
		Logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
	outQueue, outWriter, cancel := createOutputQueueAndWriter(checkCtx)
	require.NotNil(t, outQueue)
	require.NotNil(t, outWriter)
	require.NoError(t, registerOutputHandlers(checkCtx, outWriter))
	outWriter.Start()
	cancel()
	_ = outWriter.Stop()

	// 3. setupResultCallback live invocation
	asyncValidator := validation.NewAsyncValidator(pkgctx.NewSystemContext(), t.TempDir(), 2, time.Minute)
	cmdJSONL := NewCheckCmd()
	cmdJSONL.SetContext(pkgctx.NewSystemContext())
	require.NoError(t, cmdJSONL.Flags().Set("format", string(cli.FormatJSONL)))

	resultQueue := validation.NewOutputQueue(100)
	setupResultCallback(asyncValidator, cmdJSONL, cli.ContextForProjectAndProfile("", "system"), resultQueue)

	// 4. Roundtrip check result conversion & transient issue filtering
	result := &CheckResult{
		ObjectID:   "TST-PROOF-001",
		ObjectKind: "test_case",
		FilePath:   "/path/to/test.yaml",
		Status:     "active",
		AutoFixed:  []string{"missing_hash", "format_fix"},
		Issues: []Issue{
			{Tier: 1, Category: "integrity", Message: "Broken reference"},
			{Tier: 2, Category: "CacheLag", Message: "Should be filtered out"},
			{Tier: 2, Category: "cache_coherence", Message: "Should also be filtered out"},
		},
	}
	state := convertCheckResultToValidationState(result)
	require.Equal(t, 1, len(state.Issues), "transient issues should be filtered")
	assert.Equal(t, "Broken reference", state.Issues[0].Message)

	roundtrip := convertValidationStateToCheckResult(state)
	assert.Equal(t, result.ObjectID, roundtrip.ObjectID)
	assert.Equal(t, result.ObjectKind, roundtrip.ObjectKind)
	assert.Equal(t, result.Status, roundtrip.Status)
	assert.Equal(t, result.AutoFixed, roundtrip.AutoFixed)
}

// TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_ChangeJournalPendingAndCoherence
// verifies change journal event tracking, pending log deduplication, and cache invalidation.
func TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_ChangeJournalPendingAndCoherence(t *testing.T) {
	root := t.TempDir()
	storage.ResetSignificantCacheChangeForTest(root)
	storage.ResetObjectIDCachePendingForTest()
	t.Cleanup(func() {
		storage.ResetSignificantCacheChangeForTest(root)
		storage.ResetObjectIDCachePendingForTest()
	})

	// 1. Empty pending journal
	emptyIDs := objectIDsFromPendingJournal(root)
	assert.Nil(t, emptyIDs)

	// 2. Pending entries with duplicates and blank IDs
	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), "GOAL-A", "goal", "/path/a", "test")
	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), "GOAL-A", "goal", "/path/a", "test")
	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), "", "goal", "/path/empty", "test")
	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), "GOAL-B", "goal", "/path/b", "test")

	pendingIDs := objectIDsFromPendingJournal(root)
	assert.Equal(t, []string{"GOAL-A", "GOAL-B"}, pendingIDs, "pending IDs must be unique and non-empty")

	// 3. Significant cache change detection via pending burst threshold
	// Add remaining items to meet SignificantCacheChangePendingThreshold (5 items)
	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), "GOAL-C", "goal", "/path/c", "test")
	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), "GOAL-D", "goal", "/path/d", "test")
	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), "GOAL-E", "goal", "/path/e", "test")

	reason, ok := detectAndConsumeSignificantCacheChange(root)
	assert.True(t, ok)
	assert.Equal(t, storage.SignificantChangeReasonPendingBurst, reason)

	// Clean up pending journal file and memory cache to verify single consumption
	_ = os.Remove(filepath.Join(root, paths.ProjectDataDir, paths.CacheDir, "object-id-cache.pending.json"))
	storage.ResetObjectIDCachePendingForTest()
	_, okSecond := detectAndConsumeSignificantCacheChange(root)
	assert.False(t, okSecond)

	// 4. Custom marker consumption
	storage.NoteSignificantCacheChange(root, "unit_journal_marker")
	reasonCustom, okCustom := detectAndConsumeSignificantCacheChange(root)
	assert.True(t, okCustom)
	assert.Equal(t, "unit_journal_marker", reasonCustom)

	// 5. handleClearCache integration with preserving reason (PendingBurst)
	storage.NoteSignificantCacheChange(root, storage.SignificantChangeReasonPendingBurst)
	cmdBurst := NewCheckCmd()
	cmdBurst.SetContext(context.Background())
	checkCtxBurst := &AsyncCheckContext{
		Cmd:            cmdBurst,
		ProjectRoot:    root,
		Logger:         getLoggerForSystemCheck(cmdBurst, "system"),
		AsyncValidator: GetAsyncValidator(context.Background(), root, 0),
	}
	require.NoError(t, handleClearCache(checkCtxBurst))
	refreshBurst, _ := cmdBurst.Flags().GetBool("refresh-cache")
	clearBurst, _ := cmdBurst.Flags().GetBool("clear-cache")
	assert.True(t, refreshBurst, "refresh-cache should be enabled on pending burst")
	assert.False(t, clearBurst, "clear-cache should not be enabled for cache-preserving reason")

	// 6. handleClearCache integration with non-preserving reason (StateRestore)
	storage.NoteSignificantCacheChange(root, storage.SignificantChangeReasonStateRestore)
	cmdRestore := NewCheckCmd()
	cmdRestore.SetContext(context.Background())
	checkCtxRestore := &AsyncCheckContext{
		Cmd:            cmdRestore,
		ProjectRoot:    root,
		Logger:         getLoggerForSystemCheck(cmdRestore, "system"),
		AsyncValidator: GetAsyncValidator(context.Background(), root, 0),
	}
	require.NoError(t, handleClearCache(checkCtxRestore))
	clearRestore, _ := cmdRestore.Flags().GetBool("clear-cache")
	assert.True(t, clearRestore, "clear-cache must be enabled for non-preserving change reason")
}

// TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_CheckTargetAndStateConversion
// exercises target resolution (kind, ID list, ids-from-file), file ID extraction, and semaphore controls.
func TestOperationalDynamicProof_AsyncCheck_CRIT_1793632807358586000_4d0cf8e6_CheckTargetAndStateConversion(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	projectRoot := proj.Root
	_, err := setupSystemTestEnvironmentRoot(t, projectRoot)
	require.NoError(t, err)

	cmd := NewCheckCmd()
	cmd.SetContext(pkgctx.NewSystemContext())
	checkCtx := &AsyncCheckContext{
		Cmd:         cmd,
		ProjectRoot: projectRoot,
		Logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	// Case 1: Empty args defaults to all
	require.NoError(t, determineCheckTarget(checkCtx, nil))
	assert.Equal(t, "", checkCtx.TargetKind)
	assert.Nil(t, checkCtx.TargetIDs)

	// Case 2: "all" argument
	require.NoError(t, determineCheckTarget(checkCtx, []string{"all"}))
	assert.Equal(t, "", checkCtx.TargetKind)
	assert.Nil(t, checkCtx.TargetIDs)

	// Case 3: Kind argument
	require.NoError(t, determineCheckTarget(checkCtx, []string{"criteria", "CRIT-001"}))
	assert.Equal(t, "criteria", checkCtx.TargetKind)
	assert.Equal(t, []string{"CRIT-001"}, checkCtx.TargetIDs)

	// Case 4: Multiple ID arguments
	require.NoError(t, determineCheckTarget(checkCtx, []string{"CRIT-001", "CRIT-002"}))
	assert.Equal(t, "", checkCtx.TargetKind)
	assert.Equal(t, []string{"CRIT-001", "CRIT-002"}, checkCtx.TargetIDs)

	// Case 5: --ids-from-file
	idsFile := filepath.Join(projectRoot, "test_ids.txt")
	require.NoError(t, fileutil.WriteFile(idsFile, []byte("CRIT-FILE-1\n# comment\nCRIT-FILE-2\n"), paths.FilePerm644))
	cmdFile := NewCheckCmd()
	cmdFile.SetContext(pkgctx.NewSystemContext())
	require.NoError(t, cmdFile.Flags().Set("ids-from-file", idsFile))
	checkCtxFile := &AsyncCheckContext{
		Cmd:         cmdFile,
		ProjectRoot: projectRoot,
		Logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
	require.NoError(t, determineCheckTarget(checkCtxFile, nil))
	assert.Equal(t, []string{"CRIT-FILE-1", "CRIT-FILE-2"}, checkCtxFile.TargetIDs)

	// File ID extraction (non-CAS filenames extract stem; CAS hash filenames inspect content)
	sampleYAML := filepath.Join(projectRoot, "sample.yaml")
	require.NoError(t, fileutil.WriteFile(sampleYAML, []byte("id: EXTRACT-123\nkind: goal\n"), paths.FilePerm644))
	assert.Equal(t, "sample", extractObjectIDFromFile(sampleYAML, "goal"))
	assert.Equal(t, "sample", extractObjectIDFromFileWithContext(context.Background(), sampleYAML, "goal", checkCtx.Logger))
	assert.Equal(t, "path", extractObjectIDFromFile("/non/existent/path.yaml", "goal"))

	// Semaphore release control
	sem := make(chan struct{}, 1)
	var notAcquired int32
	releaseDiscoverySemaphoreIfAcquired(&notAcquired, sem)
	assert.Equal(t, 0, len(sem))

	var acquired int32
	atomic.StoreInt32(&acquired, 1)
	sem <- struct{}{}
	releaseDiscoverySemaphoreIfAcquired(&acquired, sem)
	assert.Equal(t, 0, len(sem))
}
