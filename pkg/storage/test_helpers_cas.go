package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"bufio"
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const casFlushTimeout = 5 * time.Second

// FlushListingIndexForKind waits for all pending CAS index updates for a specific kind to be processed.
// Uses the global CAS queue; prefer FlushListingIndexForProjectRoot when using TestingFactory (per-project queue).
func FlushListingIndexForKind(kind string) error {
	writeQueue := caspkg.GetGlobalListingIndexWriteQueue()
	return writeQueue.FlushKind(kind, casFlushTimeout)
}

// FlushListingIndexForProjectRoot waits for pending CAS index updates for the given project root.
// Use this in tests that create storage via TestingFactory so the test's isolated queue is flushed.
func FlushListingIndexForProjectRoot(projectRoot, kind string) error {
	q := caspkg.GetListingIndexWriteQueueForProjectRoot(projectRoot)
	return q.FlushKind(kind, casFlushTimeout)
}

// FlushAllListingIndexes waits for all pending listing-index updates across all kinds (global queue).
func FlushAllListingIndexes() error {
	writeQueue := caspkg.GetGlobalListingIndexWriteQueue()
	return writeQueue.FlushAll(casFlushTimeout)
}

// FlushAllListingIndexesForProjectRoot waits for all pending CAS index updates for the given project root (default timeout).
// Use in tests that use TestingFactory to allow temp dir cleanup without "directory not empty".
func FlushAllListingIndexesForProjectRoot(projectRoot string) error {
	return caspkg.FlushAllListingIndexesForProjectRootWithTimeout(projectRoot, casFlushTimeout)
}

// WaitForWALProcessing waits for write-behind operations to complete by checking if the WAL checkpoint stabilizes.
// This is useful in tests that need to wait for write-behind operations to complete after CLI commands.
// Returns error if checkpoint doesn't stabilize within timeout.
func WaitForWALProcessing(projectRoot string, timeout time.Duration) error {
	if projectRoot == emptyValue {
		return nil
	}
	if timeout <= 0 {
		return nil
	}

	// If the WAL file is empty (common after compaction when fully caught up), there is
	// nothing to wait for — checkpoint-based waits can otherwise spin until timeout.
	walPath := GetWALPath(projectRoot)
	if st, err := fileutil.Stat(walPath); err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return errfmt.Newf("stat WAL file").Wrap(err)
	} else if st != nil && st.Size() == 0 {
		return nil
	}

	// Wait for write-behind operations to be applied by observing WAL checkpoint progress.
	//
	// We must not rely on "checkpoint stability" alone, because during large/batched apply
	// the checkpoint may remain unchanged for long periods while work is still in-flight.
	// To avoid returning too early, we require:
	//  1) the applied sequence advances at least once beyond the initial value, then
	//  2) the checkpoint remains stable for a short window (stableThreshold).
	deadline := time.Now().Add(timeout)
	appliedStart, err := ReadAppliedSeq(projectRoot)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToReadWalCheckpoint).Wrap(err)
	}

	// Fast path: check if the WAL has already been fully applied by an exited CLI process
	walFile, err := fileutil.Open(walPath)
	if err == nil {
		scanner := bufio.NewScanner(walFile)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, maxWALLineSize)
		var maxSeq int64
		for scanner.Scan() {
			records, parseErr := parseWALLine(scanner.Bytes())
			if parseErr == nil {
				for _, rec := range records {
					if rec.Seq > maxSeq {
						maxSeq = rec.Seq
					}
				}
			}
		}
		_ = walFile.Close()
		if appliedStart >= maxSeq {
			return nil
		}
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	lastSeq := appliedStart
	stableCount := 0
	progressed := false
	const stableThreshold = 3 // 3 checks (300ms) after first progress indicates apply completed

	for {
		appliedSeq, err := ReadAppliedSeq(projectRoot)
		if err != nil {
			return errfmt.Newf(ConstStreamFailedToReadWalCheckpoint).Wrap(err)
		}
		if appliedSeq > appliedStart {
			progressed = true
		}

		if appliedSeq == lastSeq {
			stableCount++
		} else {
			lastSeq = appliedSeq
			stableCount = 0
		}

		if progressed && stableCount >= stableThreshold {
			return nil
		}

		if time.Now().After(deadline) {
			return errfmt.Errorf(ConstStreamTimeoutWaitingForWalProcessingAppliedSeqInt, appliedSeq, progressed)
		}

		select {
		case <-ticker.C:
		case <-time.After(time.Until(deadline)):
			return errfmt.Errorf(ConstStreamTimeoutWaitingForWalProcessingAppliedSeqInt, appliedSeq, progressed)
		}
	}
}

// FlushOrFail flushes the CAS index for a specific kind and project root, failing the test on error.
// Use this helper to reduce boilerplate in tests that need to wait for index updates.
func FlushOrFail(t testing.TB, projectRoot, kind string) {
	t.Helper()
	if err := FlushListingIndexForProjectRoot(projectRoot, kind); err != nil {
		t.Fatalf(ConstStreamFailedToFlushCasIndexForStrVal, kind, err)
	}
}

// defaultLeavePreliminaryStatus picks a non-preliminary status for membrane promote in tests.
// Create parks preliminary statuses on the draft plane; List/Count intentionally omit drafts.
// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
func defaultLeavePreliminaryStatus(kind string) string {
	switch kind {
	case objects.KindCriteria, objects.KindMilestone:
		return objects.ObjectStatusInProgress
	case objects.KindDocEntry:
		return "review"
	case objects.KindBacklogItem:
		// Prefer roadmap over validated: exploring→validated requires problem/acceptance
		// preconditions that bare test objects do not satisfy.
		return "roadmap"
	}
	// Prefer first non-preliminary, non-terminal status; if the kind is terminal-only
	// after origin (e.g. audit_event pending→completed), allow the first terminal.
	if loader := objects.GetGlobalLifecycleLoader(); loader != nil {
		if lc, err := loader.LoadLifecycle(kind); err == nil && lc != nil {
			var terminalLeave string
			for _, s := range lc.Statuses {
				if s.Preliminary || s.System || s.Archive || s.Value == "" {
					continue
				}
				if !s.Terminal {
					return s.Value
				}
				if terminalLeave == "" {
					terminalLeave = s.Value
				}
			}
			if terminalLeave != "" {
				return terminalLeave
			}
		}
	}
	return objects.ObjectStatusInProgress
}

// DistinctLeaveStatusForTest picks a non-preliminary leave status after create.
// Preliminary intents (e.g. exploring) promote to the kind default; non-preliminary
// intents that would collide with that default (e.g. validated on backlog_item) move
// one step further (in_progress) so GroupBy/filter tests keep distinct buckets.
func DistinctLeaveStatusForTest(kind, intended string) string {
	def := defaultLeavePreliminaryStatus(kind)
	if intended == "" || shouldUseObjectDraftPlane(kind, intended) {
		return def
	}
	if intended == def && kind == objects.KindBacklogItem {
		return objects.ObjectStatusInProgress
	}
	return intended
}

// CreateCASVisible creates obj then promotes off the draft plane so List/Count see it in CAS.
// leaveStatus empty → kind default (see defaultLeavePreliminaryStatus).
func CreateCASVisible(t testing.TB, fs ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any, leaveStatus string) {
	t.Helper()
	id, _ := obj[objects.FieldKeyID].(string)
	kind, _ := obj[objects.FieldKeyKind].(string)
	if id == "" || kind == "" {
		t.Fatalf("CreateCASVisible: id and kind required")
	}
	if objects.GetString(obj, objects.FieldKeyDescription) == "" {
		obj[objects.FieldKeyDescription] = "Substantive test description for CAS visibility test harness."
	}
	if err := fs.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("CreateCASVisible Create %s: %v", id, err)
	}
	if leaveStatus == "" {
		leaveStatus = defaultLeavePreliminaryStatus(kind)
	}
	// Already CAS-visible (non-preliminary create path) — skip promote.
	if f, ok := fs.(*FileObjectStorage); ok && !f.objectDraftPlaneExists(kind, id) {
		t.Logf("CreateCASVisible %s: objectDraftPlaneExists is FALSE! Skipping promote!", id)
		// Keep caller map aligned with durable status (avoid later Update demoting via stale exploring).
		if st := objects.GetString(obj, objects.FieldKeyStatus); st != "" {
			obj[objects.FieldKeyStatus] = st
		}
		return
	}
	t.Logf("CreateCASVisible %s: Proceeding to promote!", id)

	// Promote may hit lifecycle transition preconditions (e.g. goal→active needs milestone_ref).
	// Tests use audited break-glass so CreateCASVisible stays a reliable CAS-visibility helper.
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	promoteCtx := pkgctx.WithLifecycleBreakGlass(ctx, "CreateCASVisible test promote off draft plane")
	promoteCtx = pkgctx.WithAllowCoreObjectDelete(promoteCtx)
	if err := fs.Update(promoteCtx, secCtx, id, map[string]any{objects.FieldKeyStatus: leaveStatus}); err != nil {
		t.Fatalf("CreateCASVisible promote %s → %s: %v", id, leaveStatus, err)
	}
	obj[objects.FieldKeyStatus] = leaveStatus
}

// EnsureCASVisibleRef creates and promotes obj when id is not yet readable (setup refs in CRUD tests).
func EnsureCASVisibleRef(t testing.TB, fs ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any, leaveStatus string) {
	t.Helper()
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		t.Fatalf("EnsureCASVisibleRef: id required")
	}
	if _, err := fs.Read(ctx, secCtx, id); err == nil {
		return
	}
	CreateCASVisible(t, fs, ctx, secCtx, obj, leaveStatus)
}

// FlushAllOrFail flushes all CAS indexes for a project root, failing the test on error.
// Use this helper to reduce boilerplate in tests that touch multiple kinds.
func FlushAllOrFail(t testing.TB, projectRoot string) {
	t.Helper()
	if err := FlushAllListingIndexesForProjectRoot(projectRoot); err != nil {
		t.Fatalf(ConstStreamFailedToFlushAllCasIndexesVal, err)
	}
}
