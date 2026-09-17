//go:build !darwin

package wal

import "time"

func waitForWALProcessingEventDriven(projectRoot string, timeout time.Duration) error {
	// Fallback for non-darwin platforms: simulate delay
	time.Sleep(50 * time.Millisecond)
	return nil
}
