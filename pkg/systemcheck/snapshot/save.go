package snapshot

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SaveCheckSnapshot saves check results as a snapshot in the background.
func SaveCheckSnapshot(results []systemcheck.CheckResult, snapshotPath, projectRoot, command string, logger logging.Logger) error {
	goroutinelabels.NewGoroutine("check_snapshot_saver", fmt.Sprintf("saving check snapshot to %s", snapshotPath)).
		StartSimple(func() {
			if err := SaveCheckSnapshotBackground(results, snapshotPath, projectRoot, command, logger); err != nil {
				logging.Fluent(logger).Error("Failed to save check snapshot", err).
					Path(snapshotPath).
					Int("object_count", len(results)).
					Log()
			}
		})

	logging.Fluent(logger).Info("Started check snapshot save in background").
		Path(snapshotPath).
		Int("object_count", len(results)).
		Log()
	return nil
}

// SaveCheckSnapshotBackground performs synchronous saving of check results.
func SaveCheckSnapshotBackground(results []systemcheck.CheckResult, snapshotPath, projectRoot, command string, logger logging.Logger) error {
	if err := fileutil.MkdirAll(snapshotPath, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create snapshot directory").Wrap(err)
	}

	saveCtx := InitializeSnapshotSaveContext(results, snapshotPath, projectRoot, command, logger)

	blockingCount, warningCount, infoCount, recCount := CalculateIssueStatistics(results)
	metadata := CreateSnapshotMetadata(saveCtx, blockingCount, warningCount, infoCount, recCount)

	snapshot := systemcheck.CheckSnapshot{
		Metadata: metadata,
		Results:  results,
	}

	timestamp := metadata.Timestamp.Format("20060102-150405")
	snapshotFile, err := SaveJSONSnapshot(saveCtx, snapshot, timestamp)
	if err != nil {
		return err
	}

	CreateLatestSymlink(saveCtx, snapshotFile)

	totalIssues := blockingCount + warningCount + infoCount + recCount
	logging.Fluent(logger).Info("Check snapshot saved").
		File(snapshotFile).
		Int("objects", len(results)).
		Int("issues", totalIssues).
		Log()

	objs := ConvertResultsToObjects(results)
	SaveCompressedSnapshot(saveCtx, objs, metadata.Timestamp)

	return nil
}

// InitializeSnapshotSaveContext sets up the snapshot save context.
func InitializeSnapshotSaveContext(results []systemcheck.CheckResult, snapshotPath, projectRoot, command string, logger logging.Logger) *SnapshotSaveContext {
	return &SnapshotSaveContext{
		Results:      results,
		SnapshotPath: snapshotPath,
		ProjectRoot:  projectRoot,
		Command:      command,
		Logger:       logger,
	}
}

// CalculateIssueStatistics calculates issue statistics from results.
func CalculateIssueStatistics(results []systemcheck.CheckResult) (blockingCount, warningCount, infoCount, recCount int) {
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

// CreateSnapshotMetadata creates snapshot metadata.
func CreateSnapshotMetadata(saveCtx *SnapshotSaveContext, blockingCount, warningCount, infoCount, recCount int) systemcheck.CheckSnapshotMetadata {
	totalIssues := blockingCount + warningCount + infoCount + recCount
	return systemcheck.CheckSnapshotMetadata{
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

// SaveJSONSnapshot saves the JSON snapshot file.
func SaveJSONSnapshot(saveCtx *SnapshotSaveContext, snapshot systemcheck.CheckSnapshot, timestamp string) (string, error) {
	snapshotFile := filepath.Join(saveCtx.SnapshotPath, fmt.Sprintf("check-snapshot-%s.json", timestamp))

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", errfmt.Newf("failed to marshal snapshot").Wrap(err)
	}

	if err := fileutil.WriteFile(snapshotFile, data, paths.FilePerm644); err != nil {
		return "", errfmt.Newf("failed to write snapshot file").Wrap(err)
	}

	return snapshotFile, nil
}

// CreateLatestSymlink creates a "latest" symlink for easy access.
func CreateLatestSymlink(saveCtx *SnapshotSaveContext, snapshotFile string) {
	latestFile := filepath.Join(saveCtx.SnapshotPath, "check-snapshot-latest.json")
	_ = fileutil.Remove(latestFile)
	if err := fileutil.Symlink(filepath.Base(snapshotFile), latestFile); err != nil {
		logging.Fluent(saveCtx.Logger).Debug("Failed to create latest symlink").WithError(err).Log()
	}
}

// ConvertResultsToObjects converts CheckResults to map format for compression.
func ConvertResultsToObjects(results []systemcheck.CheckResult) []map[string]any {
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

// SaveCompressedSnapshot saves the compressed snapshot.
func SaveCompressedSnapshot(saveCtx *SnapshotSaveContext, objs []map[string]any, timestamp time.Time) {
	cs, err := storage.CreateCompressedSnapshot(objs, timestamp, saveCtx.Logger)
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

func createLatestCompressedSymlink(saveCtx *SnapshotSaveContext, compressedFile string) {
	latestCompressedFile := filepath.Join(saveCtx.SnapshotPath, "check-snapshot-latest.csnap")
	_ = fileutil.Remove(latestCompressedFile)
	if err := fileutil.Symlink(filepath.Base(compressedFile), latestCompressedFile); err != nil {
		logging.Fluent(saveCtx.Logger).Debug("Failed to create latest compressed symlink").WithError(err).Log()
	}
}

// LoadCheckSnapshot loads a check snapshot from a file.
func LoadCheckSnapshot(snapshotFile string) (*systemcheck.CheckSnapshot, error) {
	data, err := fileutil.ReadFile(snapshotFile)
	if err != nil {
		return nil, errfmt.Newf("failed to read snapshot file").Wrap(err)
	}

	var snapshot systemcheck.CheckSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, errfmt.Newf("failed to unmarshal snapshot").Wrap(err)
	}

	return &snapshot, nil
}

