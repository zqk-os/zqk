package scheduler

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCompareIssuesToBundleHealth_StaleIssueGreenLatest(t *testing.T) {
	root := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(root, paths.ProjectDataDir, paths.SchedulerSubdir)); err != nil {
		t.Fatal(err)
	}
	healthDir := JobLogsTestBundlesDir(root)
	if err := fileutil.EnsureDir(healthDir); err != nil {
		t.Fatal(err)
	}

	fp := "abcfingerprint12"
	// Older failing run for job A
	line1, _ := json.Marshal(map[string]any{
		KeyTimestamp:                "2026-03-29T01:00:00Z",
		KeyJobID:                    "SCH-run-pkg-foo-1",
		KeyTestOutcome:              "test_fail",
		KeyBundleCommandFingerprint: fp,
	})
	// Newer pass, same bundle fingerprint, different job id
	line2, _ := json.Marshal(map[string]any{
		KeyTimestamp:                "2026-03-29T02:00:00Z",
		KeyJobID:                    "SCH-run-pkg-foo-2",
		KeyTestOutcome:              "pass",
		KeyBundleCommandFingerprint: fp,
	})
	healthPath := filepath.Join(healthDir, "health.jsonl")
	if err := fileutil.WriteSecureFile(healthPath, append(append(line1, '\n'), append(line2, '\n')...)); err != nil {
		t.Fatal(err)
	}

	issuesPath := filepath.Join(root, paths.ProjectDataDir, paths.SchedulerSubdir, "issues.json")
	payload := IssuesPayload{
		Status:    issuesStatusIssues,
		UpdatedAt: "2026-03-29T01:30:00Z",
		Issues: []Issue{{
			JobID:   "SCH-run-pkg-foo-1",
			JobType: JobTypeRunWrapper,
			Error:   "exit status 1",
			At:      "2026-03-29T01:05:00Z",
		}},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(issuesPath, data); err != nil {
		t.Fatal(err)
	}

	rep, err := CompareIssuesToBundleHealth(root, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("rows: %d", len(rep.Rows))
	}
	r := rep.Rows[0]
	if r.ThatRunOutcome != "test_fail" {
		t.Errorf("that run outcome: %q", r.ThatRunOutcome)
	}
	if r.LatestForBundle != "pass" {
		t.Errorf("latest for bundle: %q", r.LatestForBundle)
	}
	if !strings.Contains(r.Interpretation, "stale") && !strings.Contains(r.Interpretation, "green") {
		t.Errorf("interpretation: %q", r.Interpretation)
	}
}
