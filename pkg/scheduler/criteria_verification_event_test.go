package scheduler

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStringSliceFromMetadata(t *testing.T) {
	meta := map[string]any{
		KeyTestBundleMetaCriteriaRefs: []string{" CRIT-a ", "CRIT-b"},
		KeyTestBundleMetaTestCaseRefs: []any{"TEST-1", "TEST-2"},
	}
	a := stringSliceFromMetadata(meta, KeyTestBundleMetaCriteriaRefs)
	if len(a) != 2 || a[0] != "CRIT-a" || a[1] != "CRIT-b" {
		t.Fatalf("criteria: %#v", a)
	}
	b := stringSliceFromMetadata(meta, KeyTestBundleMetaTestCaseRefs)
	if len(b) != 2 || b[0] != "TEST-1" {
		t.Fatalf("test_case_refs: %#v", b)
	}
	c := stringSliceFromMetadata(meta, "missing")
	if len(c) != 0 {
		t.Fatalf("missing: %#v", c)
	}
	d := stringSliceFromMetadata(map[string]any{objects.FieldKeyCriteriaRefs: "CRIT-x, CRIT-y"}, KeyTestBundleMetaCriteriaRefs)
	if len(d) != 2 || d[1] != "CRIT-y" {
		t.Fatalf("comma: %#v", d)
	}
}

func TestMaybeAppendTestBundleCriteriaVerificationEvidence_WritesJSONL(t *testing.T) {
	root := t.TempDir()
	job := &ScheduledJob{
		ID: "SCH-run-bundle-x",
		Metadata: map[string]any{
			KeyTestBundleMetaCriteriaRefs: []string{"CRIT-demo-1"},
			KeyTestBundleMetaTestCaseRefs: []string{"TEST-demo-2"},
		},
	}
	MaybeAppendTestBundleCriteriaVerificationEvidence(root, job, runWrapperTestOutcomePass, 0, "fpdemo")
	path := TestBundlesEventsFilePath(root)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatal(err)
	}
	if row[KeyEventType] != KeyEventTypeCriteriaVerificationEvidence {
		t.Fatalf("event_type: %#v", row[KeyEventType])
	}
	if row[KeyCriteriaVerificationSatisfied] != true {
		t.Fatalf("satisfied: %#v", row[KeyCriteriaVerificationSatisfied])
	}
	if row[objects.FieldKeyCriteriaRefs].([]any)[0] != "CRIT-demo-1" {
		t.Fatalf("criteria_refs: %#v", row[objects.FieldKeyCriteriaRefs])
	}
}

func TestMaybeAppendTestBundleCriteriaVerificationEvidence_NoMetadataNoFile(t *testing.T) {
	root := t.TempDir()
	job := &ScheduledJob{ID: "SCH-run-z", Metadata: map[string]any{}}
	MaybeAppendTestBundleCriteriaVerificationEvidence(root, job, runWrapperTestOutcomePass, 0, "fp")
	evPath := TestBundlesEventsFilePath(root)
	if _, err := fileutil.Stat(evPath); !fileutil.IsNotExist(err) {
		t.Fatalf("expected no events file, got %v", err)
	}
}

func TestMaybeAppendTestBundleCriteriaVerificationEvidence_TestFailNotSatisfied(t *testing.T) {
	root := t.TempDir()
	_ = fileutil.MkdirAll(filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, "scheduler", "cvs", "test-bundles"), paths.DirPerm755)
	job := &ScheduledJob{
		ID: "SCH-run-fail",
		Metadata: map[string]any{
			KeyTestBundleMetaCriteriaRefs: []string{"CRIT-x"},
		},
	}
	MaybeAppendTestBundleCriteriaVerificationEvidence(root, job, runWrapperTestOutcomeTF, 2, "fp2")
	path := TestBundlesEventsFilePath(root)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatal(err)
	}
	if row[KeyCriteriaVerificationSatisfied] != false {
		t.Fatalf("expected satisfied false, got %#v", row[KeyCriteriaVerificationSatisfied])
	}
}
