package objects

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFieldKeyConstName(t *testing.T) {
	tests := []struct {
		field string
		want  string
	}{
		{"id", "ID"},
		{"priority_plan_ref", "PriorityPlanRef"},
		{"api_endpoint", "APIEndpoint"},
		{"kind_under_test", "KindUnderTest"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if got := FieldKeyConstName(tt.field); got != tt.want {
				t.Errorf("FieldKeyConstName(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

func TestSupplementalFieldKeyNamesPreserveCrossPlanReferences(t *testing.T) {
	t.Parallel()

	for _, name := range supplementalFieldKeyNames {
		if name == FieldKeyPriorityPlanRefs {
			return
		}
	}
	t.Fatalf("supplementalFieldKeyNames must preserve %q across regeneration", FieldKeyPriorityPlanRefs)
}

func TestUnionFieldNamesFromSpecIndex(t *testing.T) {
	// Missing / empty spec index
	if got := UnionFieldNamesFromSpecIndex(nil); got != nil {
		t.Errorf("expected nil for nil SpecIndex, got %v", got)
	}

	idx := &SpecIndex{
		Kinds: map[string]SpecKindSummary{
			"task": {
				Fields: []SpecFieldSummary{
					{Name: "field_b"},
					{Name: "field_a"},
					{Name: ""}, // Empty ignored
				},
			},
			"item": {
				Fields: []SpecFieldSummary{
					{Name: "field_a"}, // Duplicate deduplicated
					{Name: "field_c"},
				},
			},
		},
	}


	names := UnionFieldNamesFromSpecIndex(idx)
	if len(names) != 3 || names[0] != "field_a" || names[1] != "field_b" || names[2] != "field_c" {
		t.Fatalf("unexpected sorted union: %v", names)
	}
}

func TestBuildUnionFieldKeyNamesAndGenerate(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, "object_specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create specsDir: %v", err)
	}

	specContent := `name: sample_task
fields:
  - name: sample_title
    type: string
  - name: sample_score
    type: int
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "sample_task.yaml"), []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write spec: %v", err)
	}

	names, err := BuildUnionFieldKeyNames(specsDir)
	if err != nil {
		t.Fatalf("BuildUnionFieldKeyNames failed: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("expected non-empty union names")
	}

	src, err := GenerateFieldKeysGoSource(specsDir)
	if err != nil {
		t.Fatalf("GenerateFieldKeysGoSource failed: %v", err)
	}
	if len(src) == 0 {
		t.Fatal("expected non-empty generated source")
	}
}
