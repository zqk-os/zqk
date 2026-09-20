package wal

import "time"

// WaitForWALProcessingEventDriven waits for write-behind operations to complete by observing
// the object WAL checkpoint file with event notifications (when supported by the OS).
//
// It is a drop-in replacement for WaitForWALProcessing(projectRoot, timeout) but avoids the
// polling loop used by the checkpoint stabilization check.
func WaitForWALProcessingEventDriven(projectRoot string, timeout time.Duration) error {
	if projectRoot == emptyValue || timeout <= 0 {
		return nil
	}
	return waitForWALProcessingEventDriven(projectRoot, timeout)
}
