package objects

import (
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestFindKindMappingsConfig_resolvesCanonicalPath ensures discovery resolves
// docs/process/_internal/configs/kind_mappings_config.yaml from the repo root.
func TestFindKindMappingsConfig_resolvesCanonicalPath(t *testing.T) {
	if testing.Short() {
		t.Skip("uses repo layout")
	}
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Skip("no go.mod in ancestry")
			return
		}
		root = parent
	}
	if err := fileutil.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileutil.Chdir(wd) }()

	p := findKindMappingsConfig()
	if p == "" {
		t.Fatal("findKindMappingsConfig returned empty")
	}
	slash := filepath.ToSlash(p)
	if !strings.Contains(slash, "_internal/configs/kind_mappings_config.yaml") {
		t.Fatalf("expected .../_internal/configs/kind_mappings_config.yaml, got %q", p)
	}
}

func TestGetKindFromDirectory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		dirName string
		want    string
	}{
		{"backlog", "backlog_item"},
		{"goals", "goal"},
		{"milestones", "milestone"},
		{"workstreams", "workstream"},
		{"priority_plans", "priority_plan"},
		{"criteria", "criteria"},
		{"requirements", "requirement"},
		{"tests", "test_case"},
		{"roadmaps", "roadmap"},
		{"decisions", "decision"},
		{"decision", ""}, // singular is not the CAS folder (decisions/)
		{"accounts", "account"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.dirName, func(t *testing.T) {
			got := GetKindFromDirectory(tt.dirName)
			if got != tt.want {
				t.Errorf("GetKindFromDirectory(%q) = %q, want %q", tt.dirName, got, tt.want)
			}
		})
	}
}

func TestGetDirectoryFromKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind string
		want string
	}{
		{"backlog_item", "backlog"},
		{"goal", "goals"},
		{"milestone", "milestones"},
		{"workstream", "workstreams"},
		{"priority_plan", "priority_plans"},
		{"criteria", "criteria"},
		{"requirement", "requirements"},
		{"test_case", "tests"},
		{"roadmap", "roadmaps"},
		{"decision", "decisions"},
		{"account", "accounts"},
		{"team_configuration", "team_configurations"},
		{"maturation_report", "maturation_reports"},
		{"organizational_change", "organizational_changes"},
		{"impact_analysis", "impact_analyses"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := GetDirectoryFromKind(tt.kind)
			if got != tt.want {
				t.Errorf("GetDirectoryFromKind(%q) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestMappingsAreBidirectional(t *testing.T) {
	t.Parallel()
	// Directories that have multiple kinds mapped to them (can't be perfectly bidirectional)
	multiKindDirectories := map[string][]string{
		FieldKeyMetrics: {"base_metric", "audit_aggregation_metric", "command_metric"},
	}

	// Test that legacy mappings are inverses and work with dynamic mapper
	for dir, kind := range legacyDirectoryToKind {
		gotDir := GetDirectoryFromKind(kind)
		if gotDir != dir {
			// Allow known legacy misspellings where the canonical kind maps
			// to a different directory name.
			// (e.g. "domain_registrys" is a legacy misspelling alias)
			if dir == "domain_registrys" {
				continue
			}
			t.Errorf("Mapping inconsistency: legacyDirectoryToKind[%q] = %q, but GetDirectoryFromKind(%q) = %q", dir, kind, kind, gotDir)
		}
	}

	for kind, dir := range legacyKindToDirectory {
		gotKind := GetKindFromDirectory(dir)
		// For directories with multiple kinds, check if the returned kind is one of the valid kinds
		if validKinds, isMultiKind := multiKindDirectories[dir]; isMultiKind {
			// Check if gotKind is one of the valid kinds for this directory
			valid := false
			for _, validKind := range validKinds {
				if gotKind == validKind {
					valid = true
					break
				}
			}
			if !valid {
				t.Errorf("Mapping inconsistency: legacyKindToDirectory[%q] = %q, but GetKindFromDirectory(%q) = %q (expected one of %v)", kind, dir, dir, gotKind, validKinds)
			}
		} else if gotKind != kind {
			// Some legacy mappings may not have corresponding object specs yet
			// (e.g., keystore_entry, auth_strategy) - skip these for now
			// They're kept in legacy mappings for backward compatibility
			if dir == "keystore" || dir == "auth_strategies" {
				t.Logf("Skipping bidirectional check for %q -> %q (no object spec yet)", kind, dir)
				continue
			}
			t.Errorf("Mapping inconsistency: legacyKindToDirectory[%q] = %q, but GetKindFromDirectory(%q) = %q", kind, dir, dir, gotKind)
		}
	}
}

func TestFoundationalKindsDirectoryMappings(t *testing.T) {
	foundationalKinds := map[string]string{
		KindMission:            "missions",
		KindVision:             "visions",
		KindStrategicContext:   "strategic_contexts",
		KindStakeholderProfile: "stakeholder_profiles",
		KindImportantDate:      "important_dates",
	}

	for kind, expectedDir := range foundationalKinds {
		dir := GetDirectoryFromKind(kind)
		if dir != expectedDir {
			t.Errorf("GetDirectoryFromKind(%q) = %q; expected %q", kind, dir, expectedDir)
		}
		revKind := GetKindFromDirectory(expectedDir)
		if revKind != kind {
			t.Errorf("GetKindFromDirectory(%q) = %q; expected %q", expectedDir, revKind, kind)
		}
	}
}
