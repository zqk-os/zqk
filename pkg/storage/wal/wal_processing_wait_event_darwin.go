//go:build darwin

package wal

import "time"

func waitForWALProcessingEventDriven(projectRoot string, timeout time.Duration) error {
	const stableThreshold = 3
	return waitForWALProcessingCheckpoint(projectRoot, timeout, stableThreshold, 100*time.Millisecond)
}
