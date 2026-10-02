package wal

import (
	"bufio"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// WaitForWALProcessingCheckpoint is a tuned checkpoint stabilization wait used by the
// cross-process durability barrier. Callers choose stableThreshold and ticker interval.
func WaitForWALProcessingCheckpoint(projectRoot string, timeout time.Duration, stableThreshold int, tickerInterval time.Duration) error {
	if projectRoot == emptyValue {
		return nil
	}
	if timeout <= 0 {
		return nil
	}
	if stableThreshold <= 0 {
		stableThreshold = 1
	}
	if tickerInterval <= 0 {
		tickerInterval = 100 * time.Millisecond
	}

	walPath := GetWALPath(projectRoot)
	if st, err := fileutil.Stat(walPath); err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return errfmt.Newf("stat WAL file").Wrap(err)
	} else if st != nil && st.Size() == 0 {
		return nil
	}

	deadline := time.Now().Add(timeout)
	appliedStart, err := ReadAppliedSeq(projectRoot)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToReadWalCheckpoint).Wrap(err)
	}

	// Fast path: check if the WAL has already been fully applied by the worker
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

	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	lastSeq := appliedStart
	stableCount := 0
	progressed := false

	for {
		appliedSeq, err := ReadAppliedSeq(projectRoot)
		if err != nil {
			return errfmt.Newf(ConstMiscFailedToReadWalCheckpoint).Wrap(err)
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
			return errfmt.Errorf(ConstMiscTimeoutWaitingForWalProcessingAppliedSeq, appliedSeq, progressed)
		}

		select {
		case <-ticker.C:
		case <-time.After(time.Until(deadline)):
			return errfmt.Errorf(ConstMiscTimeoutWaitingForWalProcessingAppliedSeq, appliedSeq, progressed)
		}
	}
}
