package storage

import (
	"bufio"
	"os"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
)

const casFlushTimeout = 5 * time.Second

// FlushListingIndexForKind waits for all pending CAS index updates for a specific kind to be processed.
// Uses the global CAS queue; prefer FlushListingIndexForProjectRoot when using TestingFactory (per-project queue).
func FlushListingIndexForKind(kind string) error {
	writeQueue := GetGlobalListingIndexWriteQueue()
	return writeQueue.FlushKind(kind, casFlushTimeout)
}

// FlushListingIndexForProjectRoot waits for pending CAS index updates for the given project root.
// Use this in tests that create storage via TestingFactory so the test's isolated queue is flushed.
func FlushListingIndexForProjectRoot(projectRoot, kind string) error {
	q := GetListingIndexWriteQueueForProjectRoot(projectRoot)
	return q.FlushKind(kind, casFlushTimeout)
}

// FlushAllListingIndexes waits for all pending listing-index updates across all kinds (global queue).
func FlushAllListingIndexes() error {
	writeQueue := GetGlobalListingIndexWriteQueue()
	return writeQueue.FlushAll(casFlushTimeout)
}

// FlushAllListingIndexesForProjectRoot waits for all pending CAS index updates for the given project root (default timeout).
// Use in tests that use TestingFactory to allow temp dir cleanup without "directory not empty".
func FlushAllListingIndexesForProjectRoot(projectRoot string) error {
	return FlushAllListingIndexesForProjectRootWithTimeout(projectRoot, casFlushTimeout)
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
	if st, err := os.Stat(walPath); err != nil {
		if os.IsNotExist(err) {
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
	walFile, err := os.Open(walPath)
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
		walFile.Close()
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

// FlushAllOrFail flushes all CAS indexes for a project root, failing the test on error.
// Use this helper to reduce boilerplate in tests that touch multiple kinds.
func FlushAllOrFail(t testing.TB, projectRoot string) {
	t.Helper()
	if err := FlushAllListingIndexesForProjectRoot(projectRoot); err != nil {
		t.Fatalf(ConstStreamFailedToFlushAllCasIndexesVal, err)
	}
}
