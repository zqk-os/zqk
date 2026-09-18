package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestParseFilterString_Equality(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantField string
		wantValue any
	}{
		{
			name:      "simple equality",
			input:     "status=active",
			wantField: "status",
			wantValue: "active",
		},
		{
			name:      "equality with spaces around operator",
			input:     "status = active",
			wantField: "status",
			wantValue: "active",
		},
		{
			name:      "double equal syntax",
			input:     "status == in_progress",
			wantField: "status",
			wantValue: "in_progress",
		},
		{
			name:      "colon syntax",
			input:     "status:active",
			wantField: "status",
			wantValue: "active",
		},
		{
			name:      "colon syntax with spaces",
			input:     "status : active",
			wantField: "status",
			wantValue: "active",
		},
		{
			name:      "kebab-case field normalization",
			input:     "priority-plan-ref=PRI-123",
			wantField: "priority_plan_ref",
			wantValue: "PRI-123",
		},
		{
			name:      "leading dashes stripped from field",
			input:     "--priority_tier=P1",
			wantField: "priority_tier",
			wantValue: "P1",
		},
		{
			name:      "quoted value with spaces",
			input:     `title="My Feature Plan"`,
			wantField: "title",
			wantValue: "My Feature Plan",
		},
		{
			name:      "pipe-separated in-list equality",
			input:     `status="active|in_progress"`,
			wantField: "status",
			wantValue: map[string]any{"$in": []string{"active", "in_progress"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, value, err := ParseFilterString(tt.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if field != tt.wantField {
				t.Errorf("field = %q, want %q", field, tt.wantField)
			}
			if !reflect.DeepEqual(value, tt.wantValue) {
				t.Errorf("value = %#v, want %#v", value, tt.wantValue)
			}
		})
	}
}

func TestParseFilterString_NotEqual(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantField string
		wantValue any
	}{
		{
			name:      "simple not-equal",
			input:     "status!=complete",
			wantField: "status",
			wantValue: map[string]any{"$ne": "complete"},
		},
		{
			name:      "not-equal with spaces",
			input:     "status != complete",
			wantField: "status",
			wantValue: map[string]any{"$ne": "complete"},
		},
		{
			name:      "pipe-separated not-in list",
			input:     `status!="complete|archived|closed"`,
			wantField: "status",
			wantValue: map[string]any{"$nin": []string{"complete", "archived", "closed"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, value, err := ParseFilterString(tt.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if field != tt.wantField {
				t.Errorf("field = %q, want %q", field, tt.wantField)
			}
			if !reflect.DeepEqual(value, tt.wantValue) {
				t.Errorf("value = %#v, want %#v", value, tt.wantValue)
			}
		})
	}
}

func TestParseFilterString_InvalidSyntax(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSubstr string
	}{
		{
			name:       "empty filter",
			input:      "   ",
			wantSubstr: "empty filter expression",
		},
		{
			name:       "missing operator",
			input:      "status in_progress",
			wantSubstr: "missing operator",
		},
		{
			name:       "single word no operator",
			input:      "status",
			wantSubstr: "missing operator",
		},
		{
			name:       "missing field name before =",
			input:      "=active",
			wantSubstr: "missing field name before = operator",
		},
		{
			name:       "missing field name before !=",
			input:      "!=complete",
			wantSubstr: "missing field name before != operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseFilterString(tt.input)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error %q does not contain expected substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestSuggestSimilarFields(t *testing.T) {
	validFields := []string{
		"id", "kind", "title", "description", "status", "priority", "priority_tier",
		"priority_plan_ref", "created_at", "created_by", "updated_at", "estimated_effort",
	}

	t.Run("exact typo (Levenshtein 1)", func(t *testing.T) {
		suggestions := SuggestSimilarFields("staus", validFields)
		if len(suggestions) == 0 || suggestions[0] != "status" {
			t.Fatalf("expected status suggestion, got: %v", suggestions)
		}
	})

	t.Run("prefix match", func(t *testing.T) {
		suggestions := SuggestSimilarFields("prio", validFields)
		if len(suggestions) == 0 {
			t.Fatalf("expected priority suggestions, got empty")
		}
		foundPriority := false
		for _, s := range suggestions {
			if strings.HasPrefix(s, "priority") {
				foundPriority = true
				break
			}
		}
		if !foundPriority {
			t.Fatalf("expected priority prefix match, got: %v", suggestions)
		}
	})

	t.Run("typo with transposition", func(t *testing.T) {
		suggestions := SuggestSimilarFields("priortiy_tier", validFields)
		if len(suggestions) == 0 || suggestions[0] != "priority_tier" {
			t.Fatalf("expected priority_tier suggestion, got: %v", suggestions)
		}
	})
}

func TestValidateFilterFields(t *testing.T) {
	t.Run("valid fields for backlog_item", func(t *testing.T) {
		filters := map[string]any{
			objects.FieldKeyStatus:   objects.ObjectStatusInProgress,
			objects.FieldKeyPriority: "high",
		}
		if err := ValidateFilterFields("backlog_item", filters); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("dotted path field", func(t *testing.T) {
		filters := map[string]any{
			"child_rollup.completed_count": 5,
		}
		// child_rollup is a valid field on priority_plan
		if err := ValidateFilterFields("priority_plan", filters); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("typo field with suggestion", func(t *testing.T) {
		filters := map[string]any{
			"staus": "active",
		}
		err := ValidateFilterFields("backlog_item", filters)
		if err == nil {
			t.Fatal("expected error for staus, got nil")
		}
		if !strings.Contains(err.Error(), `Did you mean "status"?`) {
			t.Fatalf("expected 'Did you mean \"status\"?', got: %v", err)
		}
	})
}
