package system

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// CheckSnapshot represents a snapshot of check results for analysis
type CheckSnapshot struct {
	Metadata CheckSnapshotMetadata `json:"metadata"`
	Results  []CheckResult         `json:"results"`
}

// CheckSnapshotMetadata contains metadata about the snapshot
type CheckSnapshotMetadata struct {
	Timestamp     time.Time `json:"timestamp"`
	ProjectRoot   string    `json:"project_root"`
	TotalObjects  int       `json:"total_objects"`
	TotalIssues   int       `json:"total_issues"`
	BlockingCount int       `json:"blocking_count"`
	WarningCount  int       `json:"warning_count"`
	InfoCount     int       `json:"info_count"`
	RecCount      int       `json:"rec_count"`
	Command       string    `json:"command"` // Original command that generated this snapshot
}

// SaveCheckSnapshot saves check results as a snapshot to the specified test-scenario folder
// Returns immediately and saves in the background for large result sets
func SaveCheckSnapshot(results []CheckResult, snapshotPath, projectRoot, command string, logger logging.Logger) error {
	// Start background save - return immediately
	goroutinelabels.NewGoroutine("check_snapshot_saver", fmt.Sprintf("saving check snapshot to %s", snapshotPath)).
		StartSimple(func() {
			if err := saveCheckSnapshotBackground(results, snapshotPath, projectRoot, command, logger); err != nil {
				logging.Fluent(logger).Error("Failed to save check snapshot", err).
					String("snapshot_path", snapshotPath).
					Int("object_count", len(results)).
					Log()
			}
		})

	// Return immediately - snapshot save happens in background
	logging.Fluent(logger).Info("Started check snapshot save in background").
		String("snapshot_path", snapshotPath).
		Int("object_count", len(results)).
		Log()
	return nil
}

// saveCheckSnapshotBackground performs the actual snapshot save work in the background
func saveCheckSnapshotBackground(results []CheckResult, snapshotPath, projectRoot, command string, logger logging.Logger) error {
	if err := os.MkdirAll(snapshotPath, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create snapshot directory").Wrap(err)
	}

	saveCtx := initializeSnapshotSaveContext(results, snapshotPath, projectRoot, command, logger)

	blockingCount, warningCount, infoCount, recCount := calculateIssueStatistics(results)
	metadata := createSnapshotMetadata(saveCtx, blockingCount, warningCount, infoCount, recCount)

	snapshot := CheckSnapshot{
		Metadata: metadata,
		Results:  results,
	}

	timestamp := metadata.Timestamp.Format("20060102-150405")
	snapshotFile, err := saveJSONSnapshot(saveCtx, snapshot, timestamp)
	if err != nil {
		return err
	}

	createLatestSymlink(saveCtx, snapshotFile)

	totalIssues := blockingCount + warningCount + infoCount + recCount
	logging.Fluent(logger).Info("Check snapshot saved").
		File(snapshotFile).
		Int("objects", len(results)).
		Int("issues", totalIssues).
		Log()

	objects := convertResultsToObjects(results)
	saveCompressedSnapshot(saveCtx, objects, metadata.Timestamp)

	return nil
}

// LoadCheckSnapshot loads a check snapshot from a file
func LoadCheckSnapshot(snapshotFile string) (*CheckSnapshot, error) {
	data, err := os.ReadFile(snapshotFile)
	if err != nil {
		return nil, errfmt.Newf("failed to read snapshot file").Wrap(err)
	}

	var snapshot CheckSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, errfmt.Newf("failed to unmarshal snapshot").Wrap(err)
	}

	return &snapshot, nil
}
