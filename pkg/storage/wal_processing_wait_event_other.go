//go:build !darwin

package storage

import "time"

func waitForWALProcessingEventDriven(projectRoot string, timeout time.Duration) error {
	// Fallback for non-darwin platforms: keep existing polling semantics.
	return WaitForWALProcessing(projectRoot, timeout)
}
