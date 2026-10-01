package system

import (
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/systemcheck/snapshot"
)

// ExpandCheckSnapshot loads a check snapshot JSONL file, processes it, and generates expanded output
// This is used to compare baseline check results with processed/enriched data
// Returns immediately and processes in the background for large files
func ExpandCheckSnapshot(snapshotPath, outputPath string, logger logging.Logger) error {
	return snapshot.ExpandCheckSnapshot(snapshotPath, outputPath, logger)
}

// CompareCheckOutputs compares two check output files and generates a diff
func CompareCheckOutputs(baselinePath, expandedPath, diffPath string, logger logging.Logger) error {
	return snapshot.CompareCheckOutputs(baselinePath, expandedPath, diffPath, logger)
}

// CheckDiff represents the difference between two check outputs
type CheckDiff = snapshot.CheckDiff
