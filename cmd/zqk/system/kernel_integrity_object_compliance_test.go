package system

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestLoadObjectComplianceSnapshot_Trend(t *testing.T) {
	root := t.TempDir()
	preCommit := filepath.Join(root, ".zqk", "pre-commit")
	if err := fileutil.MkdirAll(preCommit, 0o755); err != nil {
		t.Fatal(err)
	}
	checkPath := filepath.Join(preCommit, "system-check.json")
	writeCheckSummary := func(blocking, total int) {
		t.Helper()
		payload := map[string]any{
			objects.FieldKeySummary: map[string]any{
				"total_objects":           10,
				"total_issues":            total,
				"blocking_issues":         blocking,
				"warnings":                0,
				"informational":           0,
				"recommendations":         0,
				"auto_fixed":              0,
				"pending_autofix_batches": 0,
			},
		}
		b, _ := json.Marshal(payload)
		if err := fileutil.WriteFile(checkPath, b, 0o644); err != nil {
			t.Fatal(err)
		}
		// Ensure distinct mtimes across samples.
		past := time.Now().Add(-2 * time.Hour)
		_ = fileutil.Chtimes(checkPath, past, past)
	}

	writeCheckSummary(100, 120)
	snap1 := loadObjectComplianceSnapshot(root)
	if !snap1.Available || snap1.BlockingIssues != 100 {
		t.Fatalf("snap1: %+v", snap1)
	}
	if snap1.ObjectComplianceOK {
		t.Fatal("expected object_compliance_ok false when blocking>0")
	}

	// Improve counts with a newer mtime.
	payload := map[string]any{
		objects.FieldKeySummary: map[string]any{
			"total_objects":           10,
			"total_issues":            80,
			"blocking_issues":         50,
			"warnings":                0,
			"informational":           0,
			"recommendations":         0,
			"auto_fixed":              0,
			"pending_autofix_batches": 0,
		},
	}
	b, _ := json.Marshal(payload)
	if err := fileutil.WriteFile(checkPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_ = fileutil.Chtimes(checkPath, now, now)

	snap2 := loadObjectComplianceSnapshot(root)
	if snap2.Trend != "improving" {
		t.Fatalf("expected improving trend, got %q delta=%+v", snap2.Trend, snap2.Delta)
	}
	if snap2.Delta == nil || snap2.Delta.BlockingIssues != -50 {
		t.Fatalf("expected blocking delta -50, got %+v", snap2.Delta)
	}
}

func TestLoadObjectComplianceSnapshot_SkipsEmptySummary(t *testing.T) {
	root := t.TempDir()
	preCommit := filepath.Join(root, ".zqk", "pre-commit")
	if err := fileutil.MkdirAll(preCommit, 0o755); err != nil {
		t.Fatal(err)
	}
	empty := []byte(`{"summary":{"total_objects":0,"blocking_issues":0,"total_issues":0}}`)
	if err := fileutil.WriteFile(filepath.Join(preCommit, "system-check.json"), empty, 0o644); err != nil {
		t.Fatal(err)
	}
	snap := loadObjectComplianceSnapshot(root)
	if snap.Available {
		t.Fatalf("empty total_objects must not count as available: %+v", snap)
	}
}

func TestClassifyObjectComplianceTrend(t *testing.T) {
	if classifyObjectComplianceTrend(&objectComplianceDelta{BlockingIssues: -1}) != "improving" {
		t.Fatal("blocking down => improving")
	}
	if classifyObjectComplianceTrend(&objectComplianceDelta{BlockingIssues: 1}) != "worsening" {
		t.Fatal("blocking up => worsening")
	}
	if classifyObjectComplianceTrend(&objectComplianceDelta{}) != "flat" {
		t.Fatal("zero delta => flat")
	}
}
