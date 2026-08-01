package scheduler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestIsChurnStyleSchedulerJobID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id   string
		want bool
	}{
		{"", false},
		{"SCH-val", false},
		{"SCH-007", false},
		{"SCH-123", false},
		{"SCH-run-pkg-foo", false},
		{"SCH-1771968798-zqk-session-ZQK-028", true},
		{"SCH-1771968703-scheduler-job-SCH-1771968703-bucketing-strategy-BST-102", true},
		{"SCH-TEST-LOG-001", false},
	}
	for _, tc := range cases {
		if got := IsChurnStyleSchedulerJobID(tc.id); got != tc.want {
			t.Errorf("IsChurnStyleSchedulerJobID(%q) = %v; want %v", tc.id, got, tc.want)
		}
	}
}

func TestJobLogsTestBundlesDir_UnderCVSNamespace(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got := JobLogsTestBundlesDir(root)
	wantRel := filepath.Join(paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, paths.SchedulerCVSSubdir, paths.SchedulerTestBundlesSubdir)
	if !strings.HasSuffix(got, wantRel) {
		t.Fatalf("JobLogsTestBundlesDir = %q; want suffix %q", got, wantRel)
	}
}

func TestNormalizeTestBundleRedirectLogPath_legacyFlat(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	legacy := legacyFlatSchedulerTestBundlesDir(root)
	want := JobLogsTestBundlesDir(root)
	logFile := filepath.Join(legacy, "bundle-cmd-zqk-system-6-1776354311.log")
	got := NormalizeTestBundleRedirectLogPath(root, logFile)
	if got != filepath.Join(want, "bundle-cmd-zqk-system-6-1776354311.log") {
		t.Fatalf("NormalizeTestBundleRedirectLogPath = %q; want under canonical dir", got)
	}
	// Already canonical: unchanged
	canonicalPath := filepath.Join(want, "bundle-x.log")
	if p := NormalizeTestBundleRedirectLogPath(root, canonicalPath); p != canonicalPath {
		t.Fatalf("expected canonical path unchanged, got %q", p)
	}
}

func TestMergeLegacyFlatTestBundleFilesIntoCanonical(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	base := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir)
	legacy := filepath.Join(base, paths.SchedulerTestBundlesSubdir)
	canon := filepath.Join(base, paths.SchedulerCVSSubdir, paths.SchedulerTestBundlesSubdir)
	if err := fileutil.EnsureDir(legacy); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureDir(canon); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(legacy, "bundle-only-in-legacy.log")
	if err := fileutil.WriteSecureFile(oldFile, []byte("x")); err != nil {
		t.Fatal(err)
	}
	mergeLegacyFlatTestBundleFilesIntoCanonical(legacy, canon)
	if _, err := os.Stat(filepath.Join(canon, "bundle-only-in-legacy.log")); err != nil {
		t.Fatalf("expected file merged into canonical: %v", err)
	}
	if _, err := os.Stat(legacy); err == nil {
		t.Fatal("expected empty legacy dir removed")
	}
}

func TestMaybeMigrateLegacyFlatTestBundlesDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	base := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir)
	oldTB := filepath.Join(base, paths.SchedulerTestBundlesSubdir)
	if err := fileutil.EnsureDir(oldTB); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(oldTB, "health.jsonl")
	if err := fileutil.WriteSecureFile(marker, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	newTB := JobLogsTestBundlesDir(root)
	if _, err := os.Stat(filepath.Join(newTB, "health.jsonl")); err != nil {
		t.Fatalf("expected migrated health.jsonl: %v", err)
	}
	if _, err := os.Stat(oldTB); err == nil {
		t.Fatal("legacy flat test-bundles dir should be renamed away")
	}
}

func TestCVSMeasurementEventsFilePath_UnderCVSRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got := CVSMeasurementEventsFilePath(root)
	wantSuffix := filepath.Join(paths.SchedulerJobLogsSubdir, paths.SchedulerCVSSubdir, "cvs_measurement_events.jsonl")
	if !strings.HasSuffix(got, filepath.Join(paths.ProjectDataDir, paths.LogsDir, wantSuffix)) {
		t.Fatalf("CVSMeasurementEventsFilePath = %q; want suffix …/%s", got, wantSuffix)
	}
}

func TestJobLogDir_ChurnUsesSharedSubdir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	id := "SCH-1771968798-zqk-session-ZQK-028"
	got := JobLogDir(root, id)
	wantSuffix := filepath.Join(paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, paths.SchedulerChurnRunsSubdir)
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("JobLogDir = %q; want suffix %q", got, wantSuffix)
	}
}

func TestJobLogFileStem_LongIDUsesHashPrefix(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", maxSchedulerJobLogFilenameBytes+1)
	stem := JobLogFileStem(long)
	if !strings.HasPrefix(stem, "SCH-LONG-") {
		t.Fatalf("expected SCH-LONG- prefix, got %q", stem)
	}
	if stem == long {
		t.Fatal("expected stem to differ from raw id")
	}
	// Deterministic
	if JobLogFileStem(long) != stem {
		t.Fatal("stem not deterministic")
	}
}

func TestJobStdoutFilePath_UnderChurnDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	id := "SCH-99-zqk-session-ZQK-1"
	stem := JobLogFileStem(id)
	if stem == id {
		t.Fatal("churn-style job id should use a shortened log stem")
	}
	p := JobStdoutFilePath(root, id)
	if !strings.Contains(p, paths.SchedulerChurnRunsSubdir) {
		t.Fatalf("path should be under churn-runs: %s", p)
	}
	if !strings.HasSuffix(p, stem+".stdout") {
		t.Fatalf("expected suffix %s.stdout, got %s", stem, p)
	}
}

func TestJobLogFileStem_ChurnNestedSchedulerJobShort(t *testing.T) {
	t.Parallel()
	id := "SCH-1771968612-scheduler-job-SCH-1771968612-backlog-item-ITEM-102"
	stem := JobLogFileStem(id)
	if stem == id {
		t.Fatal("expected shortened stem for nested churn id")
	}
	if len(stem) > 80 {
		t.Fatalf("stem unexpectedly long (%d): %q", len(stem), stem)
	}
	if !strings.HasPrefix(stem, "SCH-1771968612-") {
		t.Fatalf("expected unix prefix preserved, got %q", stem)
	}
	if JobLogFileStem(id) != stem {
		t.Fatal("stem must be deterministic")
	}
}
