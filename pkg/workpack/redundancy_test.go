//go:build !zqk_omit_workpack

package workpack

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// Message and fixture constants for the redundancy audit tests.
const (
	msgCompletenessField      = "completeness_validation"
	msgNormalizeFieldMismatch = "NormalizeField(%q) = %q, want %q"
	msgNormalizeCollapsing    = "NormalizeField = %q, want a_b_c"
	msgBaseFieldMissing       = "base field set missing system-managed field %q (base=%v)"
	msgRedundantWant          = "Redundant = %v, want %v"
	msgRedundantUpdatedWant   = "Redundant = %v, want [updated_at]"
	msgRedundantCreatedWant   = "Redundant = %v, want [created_at]"
	msgRedundantCleanFail     = "report with redundant fields must not be clean"
	msgDuplicatesWant         = "Duplicates = %v, want [change_log]"
	msgCleanSpecFail          = "clean spec must audit clean, got %+v"
	msgGoalCleanYAML          = "goal_clean.yaml"
	msgCleanFileFail          = "clean file must audit clean, got %+v"
	msgMissingFileErr         = "expected error for missing spec file"
	msgGoalYAML               = "goal.yaml"
)

func wantList(got []string) []string {
	out := append([]string(nil), got...)
	sort.Strings(out)
	return out
}

func TestNormalizeFieldCamelToSnake(t *testing.T) {
	cases := map[string]string{
		"ArchivedAt":      "archived_at",
		"change_log":      "change_log",
		"Source-Type":     "source_type",
		"goalSummary":     "goal_summary",
		"ID":              "i_d",
		msgCompletenessField: msgCompletenessField,
	}
	for in, want := range cases {
		if got := NormalizeField(in); got != want {
			t.Errorf(msgNormalizeFieldMismatch, in, got, want)
		}
	}
}

func TestNormalizeFieldCollapsesSeparators(t *testing.T) {
	if got := NormalizeField("a__b--c"); got != "a_b_c" {
		t.Fatalf(msgNormalizeCollapsing, got)
	}
}

func TestBaseFieldSetCoversSystemManagedFields(t *testing.T) {
	base := BaseFieldSet()
	for _, f := range []string{"id", "ontology", "plane", "status", "created_at", "updated_at", "archived_at", "archived_by"} {
		if !base[f] {
			t.Fatalf(msgBaseFieldMissing, f, base)
		}
	}
}

func TestAuditSpecFlagsRedundantBaseFields(t *testing.T) {
	spec := map[string]any{
		"ontology": "goal",
		"fields": map[string]any{
			"goal_summary": map[string]any{"type": "string"},
			"archived_at":  map[string]any{"type": "string"}, // redundant: system-managed
			"Status":       map[string]any{"type": "enum"},   // redundant: normalized collision
			"owner_handle": map[string]any{"type": "string"},
		},
	}
	report := AuditSpec(spec)
	want := wantList([]string{"archived_at", "status"})
	if !reflect.DeepEqual(wantList(report.Redundant), want) {
		t.Fatalf(msgRedundantWant, report.Redundant, want)
	}
	if report.Clean() {
		t.Fatalf(msgRedundantCleanFail)
	}
}

func TestAuditSpecFlagsDuplicateFieldNames(t *testing.T) {
	spec := map[string]any{
		"ontology": "goal",
		"fields": map[string]any{
			"change_log": map[string]any{"type": "list"},
			"ChangeLog":  map[string]any{"type": "list"}, // normalized duplicate
		},
	}
	report := AuditSpec(spec)
	if !reflect.DeepEqual(wantList(report.Duplicates), wantList([]string{"change_log"})) {
		t.Fatalf(msgDuplicatesWant, report.Duplicates)
	}
}

func TestAuditSpecTopLevelFallbackForFieldlessSpecs(t *testing.T) {
	// Specs without an explicit fields map are audited on their top-level
	// keys, minus structural meta keys.
	spec := map[string]any{
		"ontology":  "goal",
		"plane":     "transactional",
		"lifecycle": "goal.yaml",
		"created_at": map[string]any{"type": "timestamp"}, // redundant base field
		"acceptance": map[string]any{"type": "string"},
	}
	report := AuditSpec(spec)
	if !reflect.DeepEqual(wantList(report.Redundant), wantList([]string{"created_at"})) {
		t.Fatalf(msgRedundantCreatedWant, report.Redundant)
	}
}

func TestAuditSpecCleanSpecIsClean(t *testing.T) {
	spec := map[string]any{
		"ontology": "goal",
		"fields": map[string]any{
			"goal_summary": map[string]any{"type": "string"},
			"acceptance":   map[string]any{"type": "string"},
		},
	}
	if report := AuditSpec(spec); !report.Clean() {
		t.Fatalf(msgCleanSpecFail, report)
	}
}

func TestAuditSpecFile(t *testing.T) {
	dir := t.TempDir()
	redundant := `ontology: goal
fields:
  goal_summary:
    type: string
  updated_at:
    type: timestamp
`
	redPath := filepath.Join(dir, msgGoalYAML)
	if err := os.WriteFile(redPath, []byte(redundant), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := AuditSpecFile(redPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wantList(report.Redundant), wantList([]string{"updated_at"})) {
		t.Fatalf(msgRedundantUpdatedWant, report.Redundant)
	}

	clean := `ontology: goal
fields:
  goal_summary:
    type: string
`
	cleanPath := filepath.Join(dir, msgGoalCleanYAML)
	if err := os.WriteFile(cleanPath, []byte(clean), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanReport, err := AuditSpecFile(cleanPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cleanReport.Clean() {
		t.Fatalf(msgCleanFileFail, cleanReport)
	}
}

func TestAuditSpecFileRejectsUnreadablePath(t *testing.T) {
	if _, err := AuditSpecFile(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal(msgMissingFileErr)
	}
}
