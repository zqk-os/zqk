package cli

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestBuildFieldCompletionCandidates_GroupableFilter(t *testing.T) {
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"requirement": {
				Kind: "requirement",
				Fields: []objects.SpecFieldSummary{
					{
						Name:      "status",
						Traits:    []string{"listable", "groupable", "filterable"},
						Groupable: true,
					},
					{
						Name:      "title",
						Traits:    []string{"listable"},
						Groupable: false,
					},
				},
			},
		},
	}

	candidates := BuildFieldCompletionCandidates(idx, "requirement", func(f objects.SpecFieldSummary) bool {
		return f.Groupable
	})

	if len(candidates) != 1 {
		t.Fatalf("expected 1 groupable candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "status" {
		t.Errorf("expected candidate name 'status', got %q", candidates[0].Name)
	}
}

func TestFormatFieldSummary_ConstraintsAreSummarized(t *testing.T) {
	field := objects.SpecFieldSummary{
		Name:         "created_at",
		Type:         "string",
		SemanticType: "timestamp",
		TimeLike:     true,
		Format:       "RFC3339",
	}

	desc := formatFieldSummary(field)
	if desc == emptyValue {
		t.Fatalf("expected non-empty description")
	}
	if !containsAll(desc, []string{"timestamp", "RFC3339"}) {
		t.Errorf("description %q does not contain expected hints", desc)
	}

	numericField := objects.SpecFieldSummary{
		Name:      "estimate_days",
		Type:      "integer",
		Numeric:   true,
		MinValue:  1,
		MaxValue:  365,
		MaxLength: 0,
	}

	numericDesc := formatFieldSummary(numericField)
	if !containsAll(numericDesc, []string{"1", "365"}) {
		t.Errorf("numeric description %q does not contain range hints", numericDesc)
	}

	titleDesc := formatFieldSummary(objects.SpecFieldSummary{
		Name:          "title",
		Type:          "string",
		MinLength:     5,
		DisplayLength: 120,
	})
	if !containsAll(titleDesc, []string{"min 5 chars", "display width 120"}) {
		t.Errorf("title description %q conflates storage and display limits", titleDesc)
	}
}

func TestBuildEnumValueCompletions_FiltersByPrefix(t *testing.T) {
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"requirement": {
				Kind: "requirement",
				Fields: []objects.SpecFieldSummary{
					{
						Name:       "status",
						EnumValues: []string{"todo", "in_progress", "done"},
					},
				},
			},
		},
	}

	values := BuildEnumValueCompletions(idx, "requirement", "status", "in_")
	if len(values) != 1 || values[0] != "in_progress" {
		t.Fatalf("expected [in_progress], got %v", values)
	}

	allValues := BuildEnumValueCompletions(idx, "requirement", "status", "")
	if len(allValues) != 3 {
		t.Fatalf("expected all 3 enum values, got %v", allValues)
	}
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
