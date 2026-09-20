package whatsnext

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestParseHumanLogClusters(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir)
	if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	humanLogPath := filepath.Join(logDir, paths.LogEventsPrefix+"human.log")
	logContent := `2026-08-13T04:17:33Z [info] Validating lifecycle event=info
2026-08-13T04:17:33Z [error] Database connection failed attempt=1
2026-08-13T04:17:34Z [error] Database connection failed attempt=2
2026-08-13T04:17:35Z [warn] High memory usage percent=90
2026-08-13T04:17:36Z [error] Unexpected EOF
`
	if err := fileutil.WriteFile(humanLogPath, []byte(logContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	got := ParseHumanLogClusters(tmpDir)
	if len(got.Errors) < 2 {
		t.Fatalf("expected ranked errors, got %#v", got.Errors)
	}
	if got.Errors[0].Message != "Database connection failed" || got.Errors[0].Count != 2 {
		t.Fatalf("expected top error Database connection failed (2), got %#v", got.Errors[0])
	}
	if len(got.Warns) != 1 || got.Warns[0].Message != "High memory usage" {
		t.Fatalf("expected High memory usage warn, got %#v", got.Warns)
	}

	digest := FormatHumanLogClusterDigest(got)
	if !strings.Contains(digest, "Database connection failed (2)") {
		t.Fatalf("digest missing top error: %q", digest)
	}
	actions := RankedHumanLogActions(got)
	if len(actions) < 1 || !strings.Contains(actions[0], "triage-log-errors") {
		t.Fatalf("expected triage-log-errors action, got %#v", actions)
	}
}

func TestProjectStewardFocus_HumanLogClusters(t *testing.T) {
	t.Parallel()
	amb := KernelAmbience{
		Available:          true,
		ObjectComplianceOK: true,
		MetricsRollup: &MetricsRollupSnapshot{
			NextAdminAction:  "triage-log-errors: top \"boom\" (3x)",
			TopErrorClusters: []IssueCluster{{Message: "boom", Count: 3}},
			TopWarnClusters:  []IssueCluster{{Message: "slow", Count: 1}},
		},
	}
	got := ProjectStewardFocus(amb, "")
	if !strings.Contains(got, "human-log clusters: errors=1 warns=1") {
		t.Fatalf("expected human-log cluster cue, got %q", got)
	}
	if !strings.Contains(got, "metrics wave action:") {
		t.Fatalf("expected metrics wave action, got %q", got)
	}
}
