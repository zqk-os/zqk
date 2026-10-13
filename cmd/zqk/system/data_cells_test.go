package system

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestValidateDataCellsFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		jsonOut              bool
		jsonEnvelope         bool
		envelopePolicyDryRun bool
		kindNonEmpty         bool
		wantErr              bool
		errMsg               string
	}{
		{
			name:                 "valid default flags",
			jsonOut:              false,
			jsonEnvelope:         false,
			envelopePolicyDryRun: false,
			kindNonEmpty:         false,
			wantErr:              false,
		},
		{
			name:                 "valid json output",
			jsonOut:              true,
			jsonEnvelope:         false,
			envelopePolicyDryRun: false,
			kindNonEmpty:         false,
			wantErr:              false,
		},
		{
			name:                 "valid json envelope",
			jsonOut:              true,
			jsonEnvelope:         true,
			envelopePolicyDryRun: false,
			kindNonEmpty:         false,
			wantErr:              false,
		},
		{
			name:                 "valid dry run alone",
			jsonOut:              false,
			jsonEnvelope:         false,
			envelopePolicyDryRun: true,
			kindNonEmpty:         false,
			wantErr:              false,
		},
		{
			name:                 "error: json-envelope without json",
			jsonOut:              false,
			jsonEnvelope:         true,
			envelopePolicyDryRun: false,
			kindNonEmpty:         false,
			wantErr:              true,
			errMsg:               "--json-envelope requires --json",
		},
		{
			name:                 "error: envelope dry run with json",
			jsonOut:              true,
			jsonEnvelope:         false,
			envelopePolicyDryRun: true,
			kindNonEmpty:         false,
			wantErr:              true,
			errMsg:               "--envelope-policy-dry-run cannot be combined",
		},
		{
			name:                 "error: envelope dry run with json envelope",
			jsonOut:              false,
			jsonEnvelope:         true,
			envelopePolicyDryRun: true,
			kindNonEmpty:         false,
			wantErr:              true,
			errMsg:               "--envelope-policy-dry-run cannot be combined",
		},
		{
			name:                 "error: envelope dry run with kind",
			jsonOut:              false,
			jsonEnvelope:         false,
			envelopePolicyDryRun: true,
			kindNonEmpty:         true,
			wantErr:              true,
			errMsg:               "--envelope-policy-dry-run cannot be combined",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateDataCellsFlags(tc.jsonOut, tc.jsonEnvelope, tc.envelopePolicyDryRun, tc.kindNonEmpty)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errMsg)
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestFilterDataCellsByKind(t *testing.T) {
	t.Parallel()

	descriptors := []datacell.CellKindDescriptor{
		{
			Kind:               "goal",
			CellID:             "goal",
			StorageProfileWire: "cas",
			ParsedProfile:      datacell.ProfileCASEntity,
		},
		{
			Kind:               "milestone",
			CellID:             "milestone",
			StorageProfileWire: "cas",
			ParsedProfile:      datacell.ProfileCASEntity,
		},
	}

	// 1. Empty kind returns all unchanged
	filtered, err := filterDataCellsByKind(descriptors, "")
	if err != nil {
		t.Fatalf("unexpected error for empty kind: %v", err)
	}
	if len(filtered) != len(descriptors) {
		t.Fatalf("expected %d descriptors, got %d", len(descriptors), len(filtered))
	}

	// 2. Known kind returns single match
	filteredGoal, err := filterDataCellsByKind(descriptors, "goal")
	if err != nil {
		t.Fatalf("unexpected error for goal: %v", err)
	}
	if len(filteredGoal) != 1 || filteredGoal[0].Kind != "goal" {
		t.Fatalf("expected 1 goal descriptor, got %+v", filteredGoal)
	}

	// 3. Unknown kind returns error
	_, err = filterDataCellsByKind(descriptors, "nonexistent_kind_xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent kind, got nil")
	}
	if !strings.Contains(err.Error(), "no data cell for kind") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestOperationalEnvelopeTableFooter(t *testing.T) {
	t.Parallel()

	// Empty descriptors
	foot := operationalEnvelopeTableFooter(nil)
	if foot != "" {
		t.Fatalf("expected empty footer for nil descriptors, got %q", foot)
	}

	// Descriptors with known and unknown profiles
	desc := []datacell.CellKindDescriptor{
		{
			Kind:          "test_kind_1",
			ParsedProfile: datacell.ProfileCASEntity,
		},
		{
			Kind:          "test_kind_2",
			ParsedProfile: datacell.ProfileCASEntity, // duplicate profile should deduplicate
		},
		{
			Kind:          "test_kind_unknown",
			ParsedProfile: datacell.StorageProfile("unknown_profile_xyz"),
		},
	}

	foot = operationalEnvelopeTableFooter(desc)
	if foot == "" {
		t.Fatal("expected non-empty footer for valid profile")
	}
	if !strings.Contains(foot, "Operational envelope") {
		t.Errorf("footer missing header: %s", foot)
	}
}

func TestTestBundleHealthSummary(t *testing.T) {
	t.Parallel()

	// 1. formatTestBundleHealthSummaryFooter with ok=false
	if s := formatTestBundleHealthSummaryFooter(testBundleHealthSummary{}, false); s != "" {
		t.Fatalf("expected empty string for ok=false, got %q", s)
	}

	// 2. formatTestBundleHealthSummaryFooter with ok=true
	sample := testBundleHealthSummary{
		LineCount:        42,
		UpdatedAtRFC3339: "2026-10-13T12:00:00Z",
	}
	formatted := formatTestBundleHealthSummaryFooter(sample, true)
	if !strings.Contains(formatted, "42 line(s)") || !strings.Contains(formatted, "2026-10-13T12:00:00Z") {
		t.Fatalf("unexpected formatted footer: %q", formatted)
	}

	// 3. loadTestBundleHealthSummary non-existent
	tmpDir := t.TempDir()
	_, ok := loadTestBundleHealthSummary(tmpDir)
	if ok {
		t.Fatal("expected ok=false for non-existent summary")
	}

	// 4. loadTestBundleHealthSummary valid
	summaryDir := filepath.Join(tmpDir, paths.ProjectDataDir, "stream_summary")
	if err := fileutil.MkdirAll(summaryDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(sample)
	if err := fileutil.WriteFile(filepath.Join(summaryDir, "test_bundle_health.json"), data, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	loaded, ok := loadTestBundleHealthSummary(tmpDir)
	if !ok {
		t.Fatal("expected ok=true for written summary")
	}
	if loaded.LineCount != 42 || loaded.UpdatedAtRFC3339 != "2026-10-13T12:00:00Z" {
		t.Fatalf("unexpected loaded summary: %+v", loaded)
	}

	// 5. testBundleHealthSummaryFooter
	foot := testBundleHealthSummaryFooter(tmpDir)
	if !strings.Contains(foot, "42 line(s)") {
		t.Fatalf("expected footer with 42 lines, got %q", foot)
	}
}

func TestBuildDataCellJSONRows(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	desc := []datacell.CellKindDescriptor{
		{
			Kind:               "goal",
			CellID:             "goal",
			StorageProfileWire: "cas",
			ParsedProfile:      datacell.ProfileCASEntity,
		},
		{
			Kind:               "custom_raw",
			CellID:             "custom_raw",
			StorageProfileWire: "",
			ParsedProfile:      datacell.StorageProfile(""),
		},
	}

	rows := buildDataCellJSONRows(tmpDir, desc)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0].Kind != "goal" || rows[0].StorageProfile != "cas" || !rows[0].StorageProfileOK {
		t.Fatalf("unexpected row 0: %+v", rows[0])
	}
	if rows[1].Kind != "custom_raw" || rows[1].StorageProfileOK {
		t.Fatalf("unexpected row 1: %+v", rows[1])
	}
}

func TestNewDataCellsCmd(t *testing.T) {
	t.Parallel()

	cmd := NewDataCellsCmd()
	if cmd == nil {
		t.Fatal("NewDataCellsCmd() returned nil")
	}
	if cmd.Use != "data-cells" {
		t.Fatalf("expected Use 'data-cells', got %q", cmd.Use)
	}
	if cmd.Flags().Lookup("json") == nil {
		t.Error("missing --json flag")
	}
	if cmd.Flags().Lookup("json-envelope") == nil {
		t.Error("missing --json-envelope flag")
	}
	if cmd.Flags().Lookup("envelope-policy-dry-run") == nil {
		t.Error("missing --envelope-policy-dry-run flag")
	}
	if cmd.Flags().Lookup("kind") == nil {
		t.Error("missing --kind flag")
	}
}

func TestRunDataCells_FlagsValidation(t *testing.T) {
	tmpDir := t.TempDir()
	if _, err := setupSystemTestEnvironmentRoot(t, tmpDir); err != nil {
		t.Fatal(err)
	}

	cmd := NewDataCellsCmd()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	// Test invalid flag combinations
	cmd.SetArgs([]string{"--json-envelope"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error executing with --json-envelope without --json")
	}

	cmd = NewDataCellsCmd()
	cmd.SetArgs([]string{"--envelope-policy-dry-run", "--json"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected error executing with --envelope-policy-dry-run and --json")
	}
}
