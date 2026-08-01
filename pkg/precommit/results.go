// Package precommit provides types and logic for the pre-commit hook that reads
// background-check results. Category files are written by scheduler jobs (lint per
// package, integrity, policy, etc.); Aggregate merges them into a single file
// with one "block" indicator that the hook reads.
package precommit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqktime"
)

const (
	preCommitDirPerm   = 0o755
	preCommitFilePerm  = 0o600
	jsonFileExt        = ".json"
	jsonIndentPrefix   = ""
	jsonIndentValue    = "  "
	lastPrefix         = ".last-"
	categorySeparator  = ": "
	defaultFailSuffix  = ": failed"
	emptyValue         = ""
	minJSONFileNameLen = 6
	jsonExtLen         = 5

	errProjectCategoryRequired = "projectRoot and category are required"
	errProjectRootRequired     = "projectRoot is required"
	errProjectResultRequired   = "projectRoot and result are required"
	errCreatePreCommitDirFmt   = "create pre-commit dir: %w"
	errRemoveFileFmt           = "remove %s: %w"
)

// CategoryResult is the result of one check category (lint, integrity, policy, docman).
type CategoryResult struct {
	OK        bool     `json:"ok"`
	Summary   string   `json:"summary,omitempty"`
	Details   []string `json:"details,omitempty"`
	Blocking  bool     `json:"blocking"`   // If true and !OK, commit is blocked
	UpdatedAt string   `json:"updated_at"` // RFC3339
}

// AggregatedResult is the single file read by the pre-commit hook.
type AggregatedResult struct {
	UpdatedAt  string                    `json:"updated_at"` // Latest of all categories
	Block      bool                      `json:"block"`      // True if any blocking category has !OK
	Categories map[string]CategoryResult `json:"categories"`
}

// CategoryDir returns the directory for category files: projectRoot/.zqk/pre-commit
func CategoryDir(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.PreCommitDir)
}

// AggregatedPath returns the path to the single results file: projectRoot/.zqk/pre-commit/results.json
func AggregatedPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.PreCommitDir, paths.PreCommitResultsFile)
}

// CategoryPath returns the path to a category file: projectRoot/.zqk/pre-commit/<category>.json
func CategoryPath(projectRoot, category string) string {
	return filepath.Join(CategoryDir(projectRoot), category+".json")
}

// LastResultPath returns the path to the staging file written by check scripts (lint, policy, integrity).
// Timer runs write here only; the pre-commit callback reads this and writes the category + aggregate.
// Path: projectRoot/.zqk/pre-commit/.last-<category>.json
func LastResultPath(projectRoot, category string) string {
	return filepath.Join(CategoryDir(projectRoot), lastPrefix+category+jsonFileExt)
}

// ReadLastResult reads the staging file written by a check script (same shape as CategoryResult).
// Used by write-result-from-last when the run was triggered with pre_commit origin.
func ReadLastResult(projectRoot, category string) (CategoryResult, error) {
	var r CategoryResult
	path := LastResultPath(projectRoot, category)
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, err
	}
	return r, nil
}

// LintOutputPath returns the path where the lint script writes full golangci-lint output for viewing.
// Use this to create backlog items from lint issues. Updated by scripts/pre-commit-lint.sh.
func LintOutputPath(projectRoot string) string {
	return filepath.Join(CategoryDir(projectRoot), "lint-output.txt")
}

// PolicyOutputPath returns the path where the policy script writes full policy-check output (logging
// and architecture compliance). Use this to see exact violations when the policy category fails.
// Updated by scripts/pre-commit-policy.sh. View with: zqk pre-commit policy-report.
func PolicyOutputPath(projectRoot string) string {
	return filepath.Join(CategoryDir(projectRoot), "policy-output.txt")
}

// IntegrityOutputPath returns the path where the integrity script writes full system check output.
// Use this to see exact violations when the integrity category fails (Tier 1 issues, hash mismatches, etc.).
// Updated by scripts/pre-commit-integrity.sh. View with: zqk pre-commit integrity-report.
func IntegrityOutputPath(projectRoot string) string {
	return filepath.Join(CategoryDir(projectRoot), "integrity-output.txt")
}

// WriteCategory writes a single category result to projectRoot/.zqk/pre-commit/<category>.json
func WriteCategory(projectRoot, category string, result CategoryResult) error {
	if projectRoot == emptyValue || category == emptyValue {
		return errors.New(errProjectCategoryRequired)
	}
	dir := CategoryDir(projectRoot)
	if err := os.MkdirAll(dir, preCommitDirPerm); err != nil {
		return errfmt.Errorf(errCreatePreCommitDirFmt, err)
	}
	path := CategoryPath(projectRoot, category)
	data, err := json.MarshalIndent(result, jsonIndentPrefix, jsonIndentValue)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, preCommitFilePerm)
}

// ReadCategory reads a category result from projectRoot/.zqk/pre-commit/<category>.json
func ReadCategory(projectRoot, category string) (CategoryResult, error) {
	var r CategoryResult
	path := CategoryPath(projectRoot, category)
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, err
	}
	return r, nil
}

// Aggregate reads all category files in projectRoot/.zqk/pre-commit/*.json (excluding results.json),
// computes Block (true if any blocking category has !OK), and writes
// projectRoot/.zqk/pre-commit/results.json
func Aggregate(projectRoot string) (*AggregatedResult, error) {
	if projectRoot == emptyValue {
		return nil, errors.New(errProjectRootRequired)
	}
	dir := CategoryDir(projectRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// No category files yet; write a "pass" result so hook doesn't block
			out := &AggregatedResult{
				UpdatedAt:  emptyValue,
				Block:      false,
				Categories: map[string]CategoryResult{},
			}
			if writeErr := WriteAggregated(projectRoot, out); writeErr != nil {
				return nil, writeErr
			}
			return out, nil
		}
		return nil, err
	}

	aggregated := &AggregatedResult{
		Categories: make(map[string]CategoryResult),
	}
	latest := emptyValue

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == paths.PreCommitResultsFile {
			continue // skip aggregated file; only merge category files
		}
		if strings.HasPrefix(name, lastPrefix) {
			continue // skip staging files; only merge category files (lint.json, policy.json, etc.)
		}
		if len(name) < minJSONFileNameLen || name[len(name)-jsonExtLen:] != jsonFileExt {
			continue
		}
		category := name[:len(name)-jsonExtLen]
		r, err := ReadCategory(projectRoot, category)
		if err != nil {
			continue
		}
		aggregated.Categories[category] = r
		if r.UpdatedAt > latest {
			latest = r.UpdatedAt
		}
		if r.Blocking && !r.OK {
			aggregated.Block = true
		}
	}
	aggregated.UpdatedAt = latest

	if err := WriteAggregated(projectRoot, aggregated); err != nil {
		return nil, err
	}
	return aggregated, nil
}

// WriteAggregated writes the aggregated result to projectRoot/.zqk/pre-commit/results.json
func WriteAggregated(projectRoot string, a *AggregatedResult) error {
	if projectRoot == emptyValue || a == nil {
		return errors.New(errProjectResultRequired)
	}
	path := AggregatedPath(projectRoot)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, preCommitDirPerm); err != nil {
		return errfmt.Errorf(errCreatePreCommitDirFmt, err)
	}
	data, err := json.MarshalIndent(a, jsonIndentPrefix, jsonIndentValue)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, preCommitFilePerm)
}

// ReadAggregated reads the single results file used by the hook
func ReadAggregated(projectRoot string) (*AggregatedResult, error) {
	if projectRoot == emptyValue {
		return nil, errors.New(errProjectRootRequired)
	}
	path := AggregatedPath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a AggregatedResult
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	if a.Categories == nil {
		a.Categories = make(map[string]CategoryResult)
	}
	return &a, nil
}

// Clear removes all category and results files under .zqk/pre-commit/ and writes a clean
// results.json with block=false so the hook allows commits. Run after resolving blockers;
// the next background script run will repopulate categories.
func Clear(projectRoot string) error {
	if projectRoot == emptyValue {
		return errors.New(errProjectRootRequired)
	}
	dir := CategoryDir(projectRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing to clear; write a pass result so hook doesn't block
			return WriteAggregated(projectRoot, &AggregatedResult{
				UpdatedAt:  zqktime.NowRFC3339UTC(),
				Block:      false,
				Categories: map[string]CategoryResult{},
			})
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return errfmt.Errorf(errRemoveFileFmt, path, err)
		}
	}
	return WriteAggregated(projectRoot, &AggregatedResult{
		UpdatedAt:  zqktime.NowRFC3339UTC(),
		Block:      false,
		Categories: map[string]CategoryResult{},
	})
}

// BlockingCategoriesSummary returns a short summary of categories that are blocking and not OK,
// for use in the hook's error message. Keys are sorted for deterministic output.
func BlockingCategoriesSummary(a *AggregatedResult) []string {
	if a == nil {
		return nil
	}
	var keys []string
	for k := range a.Categories {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		c := a.Categories[k]
		if c.Blocking && !c.OK {
			s := k + categorySeparator + c.Summary
			if s == k+categorySeparator {
				s = k + defaultFailSuffix
			}
			out = append(out, s)
		}
	}
	return out
}
