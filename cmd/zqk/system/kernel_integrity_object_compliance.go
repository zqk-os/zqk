package system

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// objectComplianceSnapshot is the instance-validation health slice of kernel integrity.
// Membrane green (composition / dangling / pipeline) is orthogonal — do not treat as "all objects healthy".
// TRACK: BLI-1785786997399161000-76ea6811
type objectComplianceSnapshot struct {
	Available             bool                   `json:"available"`
	SourcePath            string                 `json:"source_path,omitempty"`
	MeasuredAt            string                 `json:"measured_at,omitempty"`
	TotalObjects          int                    `json:"total_objects,omitempty"`
	TotalIssues           int                    `json:"total_issues,omitempty"`
	BlockingIssues        int                    `json:"blocking_issues,omitempty"`
	Warnings              int                    `json:"warnings,omitempty"`
	Informational         int                    `json:"informational,omitempty"`
	Recommendations       int                    `json:"recommendations,omitempty"`
	AutoFixed             int                    `json:"auto_fixed,omitempty"`
	PendingAutofixBatches int                    `json:"pending_autofix_batches,omitempty"`
	ObjectComplianceOK    bool                   `json:"object_compliance_ok"`
	Delta                 *objectComplianceDelta `json:"delta,omitempty"`
	Trend                 string                 `json:"trend,omitempty"` // improving | worsening | flat | unknown
	Note                  string                 `json:"note,omitempty"`
	HistoryPath           string                 `json:"history_path,omitempty"`
}

type objectComplianceDelta struct {
	VsMeasuredAt   string `json:"vs_measured_at,omitempty"`
	TotalIssues    int    `json:"total_issues"`
	BlockingIssues int    `json:"blocking_issues"`
	Warnings       int    `json:"warnings"`
	Informational  int    `json:"informational"`
}

type checkSummaryFile struct {
	Summary struct {
		TotalObjects          int `json:"total_objects"`
		TotalIssues           int `json:"total_issues"`
		BlockingIssues        int `json:"blocking_issues"`
		Warnings              int `json:"warnings"`
		Informational         int `json:"informational"`
		Recommendations       int `json:"recommendations"`
		AutoFixed             int `json:"auto_fixed"`
		PendingAutofixBatches int `json:"pending_autofix_batches"`
	} `json:"summary"`
}

type objectComplianceHistoryEntry struct {
	MeasuredAt            string `json:"measured_at"`
	SourcePath            string `json:"source_path"`
	TotalIssues           int    `json:"total_issues"`
	BlockingIssues        int    `json:"blocking_issues"`
	Warnings              int    `json:"warnings"`
	Informational         int    `json:"informational"`
	Recommendations       int    `json:"recommendations"`
	AutoFixed             int    `json:"auto_fixed"`
	PendingAutofixBatches int    `json:"pending_autofix_batches"`
	TotalObjects          int    `json:"total_objects"`
}

func kernelObjectComplianceDir(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "kernel_health")
}

func kernelObjectComplianceHistoryPath(projectRoot string) string {
	return filepath.Join(kernelObjectComplianceDir(projectRoot), "object_compliance.jsonl")
}

// preferredCheckSummaryPaths are candidate compact check JSON outputs (newest preferred first).
func preferredCheckSummaryPaths(projectRoot string) []string {
	return []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.PreCommitDir, "system-check.json"),
		filepath.Join(projectRoot, paths.ProjectDataDir, "logs", "system-check-autofix-dangling.json"),
		filepath.Join(projectRoot, paths.ProjectDataDir, "logs", "system-check.json"),
	}
}

func loadObjectComplianceSnapshot(projectRoot string) objectComplianceSnapshot {
	histPath := kernelObjectComplianceHistoryPath(projectRoot)
	snap := objectComplianceSnapshot{
		Available:   false,
		HistoryPath: histPath,
		Trend:       "unknown",
		Note:        "No compact system-check JSON found; run zqk system check … --format json -o .zqk/pre-commit/system-check.json to populate object compliance",
	}

	var (
		src  string
		file checkSummaryFile
		fi   fileutil.FileInfo
	)
	for _, p := range preferredCheckSummaryPaths(projectRoot) {
		st, err := fileutil.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		b, err := fileutil.ReadFile(p)
		if err != nil {
			continue
		}
		var parsed checkSummaryFile
		if err := json.Unmarshal(b, &parsed); err != nil {
			continue
		}
		// Ignore stubs and empty discovery (bad kind filter / background "started" payload) so they
		// cannot report false object_compliance_ok / kernel_healthy.
		if parsed.Summary.TotalObjects == 0 {
			continue
		}
		// Prefer newest mtime among readable compact check caches (stale pre-commit vs fresher log).
		if fi == nil || st.ModTime().After(fi.ModTime()) {
			src = p
			file = parsed
			fi = st
		}
	}
	if src == "" {
		return snap
	}

	s := file.Summary
	measuredAt := ""
	if fi != nil {
		measuredAt = fi.ModTime().UTC().Format(time.RFC3339)
	}
	ok := s.BlockingIssues == 0 && s.PendingAutofixBatches == 0
	snap = objectComplianceSnapshot{
		Available:             true,
		SourcePath:            src,
		MeasuredAt:            measuredAt,
		TotalObjects:          s.TotalObjects,
		TotalIssues:           s.TotalIssues,
		BlockingIssues:        s.BlockingIssues,
		Warnings:              s.Warnings,
		Informational:         s.Informational,
		Recommendations:       s.Recommendations,
		AutoFixed:             s.AutoFixed,
		PendingAutofixBatches: s.PendingAutofixBatches,
		ObjectComplianceOK:    ok,
		HistoryPath:           histPath,
		Note:                  "Instance validation rollup from last system check cache — independent of membrane_healthy",
	}

	prev := readLastObjectComplianceHistory(histPath)
	if prev != nil && prev.SourcePath == src && prev.MeasuredAt == measuredAt {
		// Same cache file/mtime: compare to prior distinct sample if present.
		prev = readPenultimateObjectComplianceHistory(histPath)
	}
	if prev != nil {
		delta := &objectComplianceDelta{
			VsMeasuredAt:   prev.MeasuredAt,
			TotalIssues:    s.TotalIssues - prev.TotalIssues,
			BlockingIssues: s.BlockingIssues - prev.BlockingIssues,
			Warnings:       s.Warnings - prev.Warnings,
			Informational:  s.Informational - prev.Informational,
		}
		snap.Delta = delta
		snap.Trend = classifyObjectComplianceTrend(delta)
	}

	_ = appendObjectComplianceHistory(projectRoot, objectComplianceHistoryEntry{
		MeasuredAt:            measuredAt,
		SourcePath:            src,
		TotalIssues:           s.TotalIssues,
		BlockingIssues:        s.BlockingIssues,
		Warnings:              s.Warnings,
		Informational:         s.Informational,
		Recommendations:       s.Recommendations,
		AutoFixed:             s.AutoFixed,
		PendingAutofixBatches: s.PendingAutofixBatches,
		TotalObjects:          s.TotalObjects,
	})
	return snap
}

func classifyObjectComplianceTrend(d *objectComplianceDelta) string {
	if d == nil {
		return "unknown"
	}
	// Blocking dominates; then total issues.
	switch {
	case d.BlockingIssues < 0 || (d.BlockingIssues == 0 && d.TotalIssues < 0):
		return "improving"
	case d.BlockingIssues > 0 || (d.BlockingIssues == 0 && d.TotalIssues > 0):
		return "worsening"
	default:
		return "flat"
	}
}

func readLastObjectComplianceHistory(histPath string) *objectComplianceHistoryEntry {
	entries := readObjectComplianceHistoryTail(histPath, 1)
	if len(entries) == 0 {
		return nil
	}
	return &entries[0]
}

func readPenultimateObjectComplianceHistory(histPath string) *objectComplianceHistoryEntry {
	entries := readObjectComplianceHistoryTail(histPath, 2)
	if len(entries) < 2 {
		return nil
	}
	// Tail order is chronological; [0]=older, [1]=newest.
	return &entries[0]
}

func readObjectComplianceHistoryTail(histPath string, n int) []objectComplianceHistoryEntry {
	b, err := fileutil.ReadFile(histPath)
	if err != nil || len(b) == 0 {
		return nil
	}
	raw := strings.Split(strings.TrimSpace(string(b)), "\n")
	var lines []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	if n > len(lines) {
		n = len(lines)
	}
	out := make([]objectComplianceHistoryEntry, 0, n)
	for i := len(lines) - n; i < len(lines); i++ {
		var e objectComplianceHistoryEntry
		if err := json.Unmarshal([]byte(lines[i]), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out
}

func appendObjectComplianceHistory(projectRoot string, entry objectComplianceHistoryEntry) error {
	dir := kernelObjectComplianceDir(projectRoot)
	if err := fileutil.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := kernelObjectComplianceHistoryPath(projectRoot)
	// Dedup: skip append when last line matches same source+mtime+counts.
	if last := readLastObjectComplianceHistory(path); last != nil &&
		last.SourcePath == entry.SourcePath &&
		last.MeasuredAt == entry.MeasuredAt &&
		last.BlockingIssues == entry.BlockingIssues &&
		last.TotalIssues == entry.TotalIssues {
		return nil
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := fileutil.OpenFile(path, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
