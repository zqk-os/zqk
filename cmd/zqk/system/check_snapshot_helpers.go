package system

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SnapshotSaveContext groups state for saving check snapshots
type SnapshotSaveContext struct {
	Results      []CheckResult
	SnapshotPath string
	ProjectRoot  string
	Command      string
	Logger       logging.Logger
}

// initializeSnapshotSaveContext sets up the snapshot save context
func initializeSnapshotSaveContext(results []CheckResult, snapshotPath, projectRoot, command string, logger logging.Logger) *SnapshotSaveContext {
	return &SnapshotSaveContext{
		Results:      results,
		SnapshotPath: snapshotPath,
		ProjectRoot:  projectRoot,
		Command:      command,
		Logger:       logger,
	}
}

// calculateIssueStatistics calculates issue statistics from results
func calculateIssueStatistics(results []CheckResult) (blockingCount, warningCount, infoCount, recCount int) {
	for _, result := range results {
		for _, issue := range result.Issues {
			switch issue.Tier {
			case 1:
				blockingCount++
			case 2:
				warningCount++
			case 3:
				infoCount++
			case 4:
				recCount++
			}
		}
	}
	return blockingCount, warningCount, infoCount, recCount
}

// createSnapshotMetadata creates snapshot metadata
func createSnapshotMetadata(saveCtx *SnapshotSaveContext, blockingCount, warningCount, infoCount, recCount int) CheckSnapshotMetadata {
	totalIssues := blockingCount + warningCount + infoCount + recCount
	return CheckSnapshotMetadata{
		Timestamp:     time.Now().UTC(),
		ProjectRoot:   saveCtx.ProjectRoot,
		TotalObjects:  len(saveCtx.Results),
		TotalIssues:   totalIssues,
		BlockingCount: blockingCount,
		WarningCount:  warningCount,
		InfoCount:     infoCount,
		RecCount:      recCount,
		Command:       saveCtx.Command,
	}
}

// saveJSONSnapshot saves the JSON snapshot file
func saveJSONSnapshot(saveCtx *SnapshotSaveContext, snapshot CheckSnapshot, timestamp string) (string, error) {
	snapshotFile := filepath.Join(saveCtx.SnapshotPath, fmt.Sprintf("check-snapshot-%s.json", timestamp))

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", errfmt.Newf("failed to marshal snapshot").Wrap(err)
	}

	if err := fileutil.WriteFile(snapshotFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Snapshot files - 0600 is acceptable
		return "", errfmt.Newf("failed to write snapshot file").Wrap(err)
	}

	return snapshotFile, nil
}

// createLatestSymlink creates a "latest" symlink for easy access
func createLatestSymlink(saveCtx *SnapshotSaveContext, snapshotFile string) {
	latestFile := filepath.Join(saveCtx.SnapshotPath, "check-snapshot-latest.json")
	_ = fileutil.Remove(latestFile)
	if err := fileutil.Symlink(filepath.Base(snapshotFile), latestFile); err != nil {
		logging.Fluent(saveCtx.Logger).Debug("Failed to create latest symlink").WithError(err).Log()
	}
}

// convertResultsToObjects converts CheckResults to map format for compression
func convertResultsToObjects(results []CheckResult) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, result := range results {
		issuesMaps := make([]map[string]any, 0, len(result.Issues))
		for _, issue := range result.Issues {
			issueMap := map[string]any{
				objects.FieldKeyTier:     issue.Tier,
				objects.FieldKeyCategory: issue.Category,
				"message":                issue.Message,
				"auto_fixable":           issue.AutoFixable,
			}
			issuesMaps = append(issuesMaps, issueMap)
		}

		obj := map[string]any{
			"object_id":                result.ObjectID,
			objects.FieldKeyObjectKind: result.ObjectKind,
			objects.FieldKeyFilePath:   result.FilePath,
			"issues":                   issuesMaps,
		}
		if len(result.AutoFixed) > 0 {
			obj["auto_fixed"] = result.AutoFixed
		}
		out = append(out, obj)
	}
	return out
}

// saveCompressedSnapshot saves the compressed snapshot
func saveCompressedSnapshot(saveCtx *SnapshotSaveContext, objects []map[string]any, timestamp time.Time) {
	cs, err := storage.CreateCompressedSnapshot(objects, timestamp, saveCtx.Logger)
	if err != nil {
		logging.Fluent(saveCtx.Logger).Warn("Failed to create compressed snapshot").WithError(err).Log()
		return
	}

	compressedFile := filepath.Join(saveCtx.SnapshotPath, fmt.Sprintf("check-snapshot-%s.csnap", timestamp.Format("20060102-150405")))
	if err := storage.WriteCompressedSnapshot(cs, compressedFile); err != nil {
		logging.Fluent(saveCtx.Logger).Warn("Failed to write compressed snapshot").WithError(err).Log()
		return
	}

	createLatestCompressedSymlink(saveCtx, compressedFile)
	logging.Fluent(saveCtx.Logger).Info("Compressed snapshot saved").
		File(compressedFile).
		Int("objects", len(saveCtx.Results)).
		Log()
}

// createLatestCompressedSymlink creates a "latest" symlink for compressed snapshot
func createLatestCompressedSymlink(saveCtx *SnapshotSaveContext, compressedFile string) {
	latestCompressedFile := filepath.Join(saveCtx.SnapshotPath, "check-snapshot-latest.csnap")
	_ = fileutil.Remove(latestCompressedFile)
	if err := fileutil.Symlink(filepath.Base(compressedFile), latestCompressedFile); err != nil {
		logging.Fluent(saveCtx.Logger).Debug("Failed to create latest compressed symlink").WithError(err).Log()
	}
}
