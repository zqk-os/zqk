package snapshot

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ExpandCheckSnapshot loads a check snapshot JSONL file, processes it, and generates expanded output.
// Returns immediately and processes in the background for large files.
func ExpandCheckSnapshot(snapshotPath, outputPath string, logger logging.Logger) error {
	goroutinelabels.NewGoroutine("check_snapshot_expander", fmt.Sprintf("expanding check snapshot %s", snapshotPath)).
		StartSimple(func() {
			if err := ExpandCheckSnapshotBackground(snapshotPath, outputPath, logger); err != nil {
				logging.Fluent(logger).Error("Check snapshot expansion failed", err).
					String("input", snapshotPath).
					String("output", outputPath).
					Log()
			}
		})

	logging.Fluent(logger).Info("Started check snapshot expansion in background").
		String("input", snapshotPath).
		String("output", outputPath).
		Log()
	return nil
}

// ExpandCheckSnapshotBackground performs the actual expansion work synchronously.
func ExpandCheckSnapshotBackground(snapshotPath, outputPath string, logger logging.Logger) error {
	file, err := fileutil.Open(snapshotPath)
	if err != nil {
		return errfmt.Newf("failed to open snapshot file").Wrap(err)
	}
	defer file.Close()

	outFile, err := fileutil.Create(outputPath)
	if err != nil {
		return errfmt.Newf("failed to create output file").Wrap(err)
	}
	defer outFile.Close()

	decoder := json.NewDecoder(file)
	encoder := json.NewEncoder(outFile)
	encoder.SetEscapeHTML(false)

	processedCount := 0
	skippedCount := 0

	for {
		var result systemcheck.CheckResult
		if err := decoder.Decode(&result); err != nil {
			if err == io.EOF {
				break
			}
			if skippedCount == 0 {
				logging.Fluent(logger).Warn("Skipping malformed line in snapshot (likely corruption)").WithError(err).Log()
			}
			skippedCount++
			continue
		}

		enriched := EnrichCheckResult(&result)
		if err := encoder.Encode(enriched); err != nil {
			return errfmt.Newf("failed to encode result").Wrap(err)
		}

		processedCount++
		if processedCount%1000 == 0 {
			_ = outFile.Sync()
		}
	}

	_ = outFile.Sync()

	logging.Fluent(logger).Info("Expanded check snapshot").
		String("input", snapshotPath).
		String("output", outputPath).
		Int("processed", processedCount).
		Int("skipped", skippedCount).
		Log()

	return nil
}

// EnrichCheckResult enriches a check result with additional context.
func EnrichCheckResult(result *systemcheck.CheckResult) *systemcheck.CheckResult {
	return result
}
