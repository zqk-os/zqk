package snapshot

import (
	"encoding/json"
	"io"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CompareCheckOutputs compares two check output files and generates a diff.
func CompareCheckOutputs(baselinePath, expandedPath, diffPath string, logger logging.Logger) error {
	baseline, err := LoadCheckResults(baselinePath)
	if err != nil {
		return errfmt.Newf("failed to load baseline").Wrap(err)
	}

	expanded, err := LoadCheckResults(expandedPath)
	if err != nil {
		return errfmt.Newf("failed to load expanded").Wrap(err)
	}

	diff := CheckDiff{
		BaselineCount: len(baseline),
		ExpandedCount: len(expanded),
		Added:         []systemcheck.CheckResult{},
		Removed:       []systemcheck.CheckResult{},
		Modified:      []systemcheck.CheckResultDiff{},
	}

	baselineMap := make(map[string]systemcheck.CheckResult, len(baseline))
	for _, result := range baseline {
		baselineMap[result.ObjectID] = result
	}

	expandedMap := make(map[string]systemcheck.CheckResult, len(expanded))
	for _, result := range expanded {
		expandedMap[result.ObjectID] = result
	}

	for id, expandedResult := range expandedMap {
		if baselineResult, exists := baselineMap[id]; exists {
			if !ResultsEqual(&baselineResult, &expandedResult) {
				diff.Modified = append(diff.Modified, systemcheck.CheckResultDiff{
					ObjectID: id,
					Baseline: baselineResult,
					Expanded: expandedResult,
				})
			}
		} else {
			diff.Added = append(diff.Added, expandedResult)
		}
	}

	for id, baselineResult := range baselineMap {
		if _, exists := expandedMap[id]; !exists {
			diff.Removed = append(diff.Removed, baselineResult)
		}
	}

	data, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal diff").Wrap(err)
	}

	if err := fileutil.WriteFile(diffPath, data, paths.FilePerm644); err != nil {
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

// LoadCheckResults loads check results from a JSONL file.
func LoadCheckResults(filePath string) ([]systemcheck.CheckResult, error) {
	file, err := fileutil.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var results []systemcheck.CheckResult
	decoder := json.NewDecoder(file)

	for {
		var result systemcheck.CheckResult
		if err := decoder.Decode(&result); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// ResultsEqual compares two check results for equality.
func ResultsEqual(a, b *systemcheck.CheckResult) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.ObjectID != b.ObjectID {
		return false
	}
	if len(a.Issues) != len(b.Issues) {
		return false
	}
	return true
}
