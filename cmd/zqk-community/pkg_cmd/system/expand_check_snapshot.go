package system

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// ExpandCheckSnapshot loads a check snapshot JSONL file, processes it, and generates expanded output
// This is used to compare baseline check results with processed/enriched data
// Returns immediately and processes in the background for large files
func ExpandCheckSnapshot(snapshotPath, outputPath string, logger logging.Logger) error {
	// Start background expansion - return immediately
	goroutinelabels.NewGoroutine("check_snapshot_expander", fmt.Sprintf("expanding check snapshot %s", snapshotPath)).
		StartSimple(func() {
			if err := expandCheckSnapshotBackground(snapshotPath, outputPath, logger); err != nil {
				logging.Fluent(logger).Error("Check snapshot expansion failed", err).
					String("input", snapshotPath).
					String("output", outputPath).
					Log()
			}
		})

	// Return immediately - expansion happens in background
	logging.Fluent(logger).Info("Started check snapshot expansion in background").
		String("input", snapshotPath).
		String("output", outputPath).
		Log()
	return nil
}

// expandCheckSnapshotBackground performs the actual expansion work in the background
func expandCheckSnapshotBackground(snapshotPath, outputPath string, logger logging.Logger) error {
	// Open the snapshot file
	file, err := os.Open(snapshotPath)
	if err != nil {
		return errfmt.Newf("failed to open snapshot file").Wrap(err)
	}
	defer file.Close()

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return errfmt.Newf("failed to create output file").Wrap(err)
	}
	defer outFile.Close()

	decoder := json.NewDecoder(file)
	encoder := json.NewEncoder(outFile)
	encoder.SetEscapeHTML(false)

	processedCount := 0
	skippedCount := 0

	// Process each line (JSONL format - one JSON object per line)
	// Stream processing for large files
	for {
		var result CheckResult
		if err := decoder.Decode(&result); err != nil {
			if err == io.EOF {
				break
			}
			// Skip malformed lines (like the corrupted first line)
			if skippedCount == 0 {
				logging.Fluent(logger).Warn("Skipping malformed line in snapshot (likely corruption)").WithError(err).Log()
			}
			skippedCount++
			continue
		}

		// Process/enrich the result
		// For now, we just pass through, but this is where we would:
		// - Resolve references
		// - Add context
		// - Enrich with spec information
		// - Apply violation resolver logic
		enriched := enrichCheckResult(&result)

		// Write to output (streaming - flush periodically)
		if err := encoder.Encode(enriched); err != nil {
			return errfmt.Newf("failed to encode result").Wrap(err)
		}

		processedCount++

		// Sync periodically for large files (every 1000 objects)
		if processedCount%1000 == 0 {
			_ = outFile.Sync() //nolint:errcheck // Best effort - sync errors are non-critical
		}
	}

	// Final sync
	_ = outFile.Sync() //nolint:errcheck // Best effort - sync errors are non-critical

	logging.Fluent(logger).Info("Expanded check snapshot").
		String("input", snapshotPath).
		String("output", outputPath).
		Int("processed", processedCount).
		Int("skipped", skippedCount).
		Log()

	return nil
}

// enrichCheckResult enriches a check result with additional context
// This is a placeholder for future enrichment logic
func enrichCheckResult(result *CheckResult) *CheckResult {
	// For now, just return as-is
	// TODO: Add enrichment logic:
	// - Resolve object references
	// - Add spec context
	// - Apply violation resolver
	// - Add resolution suggestions
	return result
}

// CompareCheckOutputs compares two check output files and generates a diff
func CompareCheckOutputs(baselinePath, expandedPath, diffPath string, logger logging.Logger) error {
	// Load baseline
	baseline, err := loadCheckResults(baselinePath)
	if err != nil {
		return errfmt.Newf("failed to load baseline").Wrap(err)
	}

	// Load expanded
	expanded, err := loadCheckResults(expandedPath)
	if err != nil {
		return errfmt.Newf("failed to load expanded").Wrap(err)
	}

	// Create diff
	diff := CheckDiff{
		BaselineCount: len(baseline),
		ExpandedCount: len(expanded),
		Added:         []CheckResult{},
		Removed:       []CheckResult{},
		Modified:      []CheckResultDiff{},
	}

	// Build maps for comparison
	baselineMap := make(map[string]CheckResult)
	for _, result := range baseline {
		baselineMap[result.ObjectID] = result
	}

	expandedMap := make(map[string]CheckResult)
	for _, result := range expanded {
		expandedMap[result.ObjectID] = result
	}

	// Find added and modified
	for id, expandedResult := range expandedMap {
		if baselineResult, exists := baselineMap[id]; exists {
			// Check if modified
			if !resultsEqual(&baselineResult, &expandedResult) {
				diff.Modified = append(diff.Modified, CheckResultDiff{
					ObjectID: id,
					Baseline: baselineResult,
					Expanded: expandedResult,
				})
			}
		} else {
			// Added
			diff.Added = append(diff.Added, expandedResult)
		}
	}

	// Find removed
	for id, baselineResult := range baselineMap {
		if _, exists := expandedMap[id]; !exists {
			diff.Removed = append(diff.Removed, baselineResult)
		}
	}

	// Write diff
	data, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal diff").Wrap(err)
	}

	if err := os.WriteFile(diffPath, data, paths.FilePerm644); err != nil { //nolint:gosec // Diff files - 0600 is acceptable
		return errfmt.Newf("failed to write diff file").Wrap(err)
	}

	logging.Fluent(logger).Info("Generated check output comparison").
		String("baseline", baselinePath).
		String("expanded", expandedPath).
		String("diff", diffPath).
		Int("baseline_count", diff.BaselineCount).
		Int("expanded_count", diff.ExpandedCount).
		Int("added", len(diff.Added)).
		Int("removed", len(diff.Removed)).
		Int("modified", len(diff.Modified)).
		Log()

	return nil
}

// CheckDiff represents the difference between two check outputs
type CheckDiff struct {
	BaselineCount int               `json:"baseline_count"`
	ExpandedCount int               `json:"expanded_count"`
	Added         []CheckResult     `json:"added"`
	Removed       []CheckResult     `json:"removed"`
	Modified      []CheckResultDiff `json:"modified"`
}

// CheckResultDiff represents a modified check result
type CheckResultDiff struct {
	ObjectID string      `json:"object_id"`
	Baseline CheckResult `json:"baseline"`
	Expanded CheckResult `json:"expanded"`
}

// loadCheckResults loads check results from a JSONL file
func loadCheckResults(filePath string) ([]CheckResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var results []CheckResult
	decoder := json.NewDecoder(file)

	for {
		var result CheckResult
		if err := decoder.Decode(&result); err != nil {
			if err == io.EOF {
				break
			}
			// Skip malformed lines
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// resultsEqual compares two check results for equality
func resultsEqual(a, b *CheckResult) bool {
	if a.ObjectID != b.ObjectID {
		return false
	}
	if len(a.Issues) != len(b.Issues) {
		return false
	}
	// Simple comparison - could be enhanced
	return true
}
