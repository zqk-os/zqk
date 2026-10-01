package system

import (
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	"github.com/zqk-os/zqk/pkg/systemcheck/snapshot"
)

// SaveCheckSnapshot saves check results as a snapshot to the specified test-scenario folder
// Returns immediately and saves in the background for large result sets
func SaveCheckSnapshot(results []CheckResult, snapshotPath, projectRoot, command string, logger logging.Logger) error {
	return snapshot.SaveCheckSnapshot(results, snapshotPath, projectRoot, command, logger)
}

// LoadCheckSnapshot loads a check snapshot from a file
func LoadCheckSnapshot(snapshotFile string) (*systemcheck.CheckSnapshot, error) {
	return snapshot.LoadCheckSnapshot(snapshotFile)
}
