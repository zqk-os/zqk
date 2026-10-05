package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_JobTriggerQueue_MeshLease(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// 1. Classification & ID helper functions
	if !batchContainsTestBundleJobID([]JobTriggerRequest{{JobID: TestBundleJobIDPrefix + "123"}}) {
		t.Errorf("expected batchContainsTestBundleJobID true")
	}
	if batchContainsTestBundleJobID([]JobTriggerRequest{{JobID: "SCH-other"}}) {
		t.Errorf("expected batchContainsTestBundleJobID false for SCH-other")
	}

	// 10 digits
	if !jobIDLooksLikeCrossProcessTimestampTestRunner("SCH-1700000000") {
		t.Errorf("expected 10-digit cross-process timestamp to match")
	}
	if jobIDLooksLikeCrossProcessTimestampTestRunner("SCH-123") {
		t.Errorf("expected short id not to match cross-process timestamp")
	}

	// 16-19 digits
	if !jobIDLooksLikeCLISubmitNanos("SCH-1700000000123456") {
		t.Errorf("expected 16-digit nanos to match")
	}
	if jobIDLooksLikeCLISubmitNanos("SCH-1700000000") {
		t.Errorf("expected 10-digit not to match nanos")
	}

	if !jobIDAllDigitSuffixLen("SCH-12345", 5, 5) {
		t.Errorf("expected jobIDAllDigitSuffixLen true")
	}
	if jobIDAllDigitSuffixLen("NOTSCH-123", 1, 10) {
		t.Errorf("expected false for missing prefix")
	}
	if jobIDAllDigitSuffixLen("SCH-12a45", 5, 5) {
		t.Errorf("expected false for non-digit")
	}

	// triggerWarrantsCacheMissRetry
	rBundle := JobTriggerRequest{JobID: TestBundleJobIDPrefix + "abc"}
	if !triggerWarrantsCacheMissRetry(rBundle) {
		t.Errorf("expected retry for bundle")
	}
	rCLI := JobTriggerRequest{JobID: "SCH-1", TriggerOrigin: TriggerOriginCLISubmit}
	if !triggerWarrantsCacheMissRetry(rCLI) || !triggerIsCLIOneShot(rCLI) {
		t.Errorf("expected retry & one-shot for CLI submit")
	}
	rPre := JobTriggerRequest{JobID: "SCH-1", TriggerOrigin: TriggerOriginPreCommit}
	if !triggerWarrantsCacheMissRetry(rPre) || !triggerIsCLIOneShot(rPre) {
		t.Errorf("expected retry & one-shot for PreCommit")
	}

	// jobIDLooksLikeCASInstanceSchedulerJobID
	casID := "SCH-1700000000000-abcdef1234"
	if !jobIDLooksLikeCASInstanceSchedulerJobID(casID) {
		t.Errorf("expected casID to match CASInstanceSchedulerJobID")
	}
	if jobIDLooksLikeCASInstanceSchedulerJobID("SCH-simple") {
		t.Errorf("expected false for SCH-simple")
	}

	// prioritizeCachePrewarmTriggers
	reqs := []JobTriggerRequest{
		{JobID: "SCH-job-1"},
		{JobID: DefaultCachePrewarmJobID},
		{JobID: "SCH-job-2"},
	}
	prio := prioritizeCachePrewarmTriggers(reqs)
	if len(prio) != 3 || prio[0].JobID != DefaultCachePrewarmJobID {
		t.Errorf("expected DefaultCachePrewarmJobID first, got %v", prio[0].JobID)
	}
	if len(prioritizeCachePrewarmTriggers(reqs[:1])) != 1 {
		t.Errorf("expected single request unchanged")
	}

	// batchNeedsCASReconcileBeforeTriggerReload
	if !batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{rBundle}) {
		t.Errorf("expected reconcile for bundle")
	}
	if !batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{{JobID: "SCH-1700000000"}}) {
		t.Errorf("expected reconcile for cross process timestamp")
	}
	if !batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{{JobID: casID}}) {
		t.Errorf("expected reconcile for cas instance")
	}
	if !batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{rCLI}) {
		t.Errorf("expected reconcile for cli submit")
	}
	if batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{{JobID: "SCH-regular"}}) {
		t.Errorf("expected false for regular job")
	}

	// extractJobIDs
	ids := extractJobIDs([]JobTriggerRequest{{JobID: "j1"}, {JobID: "j2"}})
	if len(ids) != 2 || ids[0] != "j1" || ids[1] != "j2" {
		t.Errorf("unexpected extracted ids: %v", ids)
	}

	// 2. JobTriggerQueue lifecycle & enqueue/dequeue
	queue := NewJobTriggerQueue(tmpDir).(*JobTriggerQueue)
	queue.SetCASDebounceInterval(10 * time.Millisecond)

	if !queue.shouldReconcileCASForBatch(true, true) {
		t.Errorf("expected reconcile for missing job with needsCAS")
	}
	if queue.shouldReconcileCASForBatch(true, false) {
		t.Errorf("expected no reconcile when needsCAS is false")
	}
	if !queue.shouldReconcileCASForBatch(false, true) {
		t.Errorf("expected reconcile for needsCAS when initial lastCASReconcileAt is zero")
	}
	queue.markCASReconciled()
	// debounce should prevent immediate second reconcile
	if queue.shouldReconcileCASForBatch(false, true) {
		t.Errorf("expected debounced reconcile to be false")
	}

	// EnqueueTriggerRequest
	err := queue.EnqueueTriggerRequest("SCH-t1")
	if err != nil {
		t.Errorf("EnqueueTriggerRequest failed: %v", err)
	}
	err = queue.EnqueueTriggerRequestWithOrigin("SCH-t2", TriggerOriginCLISubmit)
	if err != nil {
		t.Errorf("EnqueueTriggerRequestWithOrigin failed: %v", err)
	}
	err = queue.EnqueueTriggerRequests([]string{"SCH-t3", "SCH-t4"}, TriggerOriginPreCommit)
	if err != nil {
		t.Errorf("EnqueueTriggerRequests failed: %v", err)
	}
	err = queue.EnqueueTriggerRequestStructs([]JobTriggerRequest{{JobID: "SCH-t5"}})
	if err != nil {
		t.Errorf("EnqueueTriggerRequestStructs failed: %v", err)
	}

	// HasPendingTriggerWithOrigin
	has, err := queue.HasPendingTriggerWithOrigin("SCH-t2", TriggerOriginCLISubmit)
	if err != nil || !has {
		t.Errorf("expected HasPendingTriggerWithOrigin true, got %v, err: %v", has, err)
	}
	hasNo, _ := queue.HasPendingTriggerWithOrigin("SCH-t2", "other")
	if hasNo {
		t.Errorf("expected HasPendingTriggerWithOrigin false for other origin")
	}

	// Peek
	peeked, err := queue.PeekTriggerRequests()
	if err != nil || len(peeked) < 5 {
		t.Errorf("PeekTriggerRequests failed: len=%d, err=%v", len(peeked), err)
	}

	// Dequeue
	deq, err := queue.DequeueTriggerRequests(2)
	if err != nil || len(deq) != 2 {
		t.Errorf("DequeueTriggerRequests failed: len=%d, err=%v", len(deq), err)
	}

	// EnqueueLifecycleTrigger
	err = queue.EnqueueLifecycleTrigger("test_kind", "pending", "running", map[string]any{"id": "obj-1"})
	if err != nil {
		t.Errorf("EnqueueLifecycleTrigger failed: %v", err)
	}
}

func TestExtended_MeshLeaseAndRunWrapper_Helpers(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = sp.Shutdown(ctx) }()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. MeshLease helpers
	if getFloat(float64(3.14)) != 3.14 {
		t.Errorf("unexpected getFloat for float64")
	}
	if getFloat(float32(2.5)) != 2.5 {
		t.Errorf("unexpected getFloat for float32")
	}
	if getFloat(int(42)) != 42.0 {
		t.Errorf("unexpected getFloat for int")
	}
	if getFloat(int64(100)) != 100.0 {
		t.Errorf("unexpected getFloat for int64")
	}
	if getFloat("invalid") != 0.0 {
		t.Errorf("unexpected getFloat for string")
	}

	meshHandler := &MeshLeaseSupervisionHandler{
		storage:     sp,
		projectRoot: tmpDir,
		logger:      logger,
	}
	meshHandler.reapStaleSubprocesses(map[string]bool{"lease-active": true})

	// 2. RunWrapper helper functions
	// stripShellOutputRedirect
	s1, t1 := stripShellOutputRedirect("echo hello > /tmp/out.log 2>&1")
	if s1 != "echo hello" || t1 != "/tmp/out.log" {
		t.Errorf("unexpected stripShellOutputRedirect >: %s, %s", s1, t1)
	}
	s2, t2 := stripShellOutputRedirect("echo hello >> /tmp/out.log 2>&1")
	if s2 != "echo hello" || t2 != "/tmp/out.log" {
		t.Errorf("unexpected stripShellOutputRedirect >>: %s, %s", s2, t2)
	}
	s3, t3 := stripShellOutputRedirect("echo hello")
	if s3 != "echo hello" || t3 != "" {
		t.Errorf("unexpected stripShellOutputRedirect no redirect: %s, %s", s3, t3)
	}

	// copyStreamFilesToRedirectTarget
	logDir := JobLogDir(tmpDir, "SCH-job-redirect")
	_ = os.MkdirAll(logDir, 0755)
	stem := JobLogFileStem("SCH-job-redirect")
	_ = os.WriteFile(filepath.Join(logDir, stem+".stdout"), []byte("standard output"), 0644)
	_ = os.WriteFile(filepath.Join(logDir, stem+".stderr"), []byte("standard error"), 0644)
	targetPath := filepath.Join(tmpDir, "combined.log")

	copyStreamFilesToRedirectTarget(tmpDir, logDir, "SCH-job-redirect", targetPath)
	content, err := os.ReadFile(targetPath)
	if err != nil || len(content) == 0 {
		t.Errorf("expected combined redirect file written: err=%v, len=%d", err, len(content))
	}
	// copyStreamFilesToRedirectTarget with empty strings
	copyStreamFilesToRedirectTarget("", "", "", "")

	// writeSeparateJobLogsIfConfigured
	writeSeparateJobLogsIfConfigured(tmpDir, "SCH-job-separate", "out", "err")

	// RunWrapperHandler panic & prep helpers
	rwh := &RunWrapperHandler{
		storage:     sp,
		projectRoot: tmpDir,
		logger:      logger,
	}

	testJob := &ScheduledJob{
		ID:          "SCH-job-prep",
		Command:     "echo 'prep test'",
		CommandArgs: []string{"arg1"},
	}
	prep := rwh.prepareRunWrapperExecution(testJob)
	if prep.CmdStr == "" {
		t.Errorf("expected non-empty CmdStr from prepareRunWrapperExecution")
	}

	rwh.warnIfTestBundleMetadataFingerprintMismatch(testJob, prep.CmdStr)
	rwh.logRunWrapperExecutionStart(testJob, prep)

	errPanic := rwh.runWrapperHandlePanic(testJob, "simulated panic", []byte("stack trace"))
	if errPanic == nil {
		t.Errorf("expected error from runWrapperHandlePanic")
	}
}
