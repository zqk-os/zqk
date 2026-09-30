package compose

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestEvalRefuseUnknownFields_CompositionFieldsAllowed(t *testing.T) {
	t.Parallel()

	obj := map[string]any{
		"id":              "BLI-001",
		"kind":            objects.KindBacklogItem,
		"claimed_by":      "ACC-123",
		"version_context": "default",
	}

	errs := evalRefuseUnknownFields(objects.KindBacklogItem, obj, nil)
	for _, e := range errs {
		if e.Field == "claimed_by" || e.Field == "version_context" {
			t.Errorf("composition field %s was incorrectly flagged as unknown: %v", e.Field, e)
		}
	}

	obj["nonexistent_garbage_field_xyz"] = "bad"
	errs = evalRefuseUnknownFields(objects.KindBacklogItem, obj, nil)
	found := false
	for _, e := range errs {
		if e.Field == "nonexistent_garbage_field_xyz" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected nonexistent_garbage_field_xyz to be flagged as unknown, got: %v", errs)
	}
}

func TestEvalPredicateDSL(t *testing.T) {
	t.Parallel()

	// 1. field_nonempty
	ruleNonEmpty := Rule{
		ID: "test_nonempty",
		Op: OpPredicateDSL,
		Config: map[string]any{
			"expression": "field_nonempty:title",
		},
	}
	errs := evalOverlayRule(nil, "backlog_item", map[string]any{"title": "Valid"}, nil, ruleNonEmpty)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for populated title, got: %v", errs)
	}
	errs = evalOverlayRule(nil, "backlog_item", map[string]any{"title": ""}, nil, ruleNonEmpty)
	if len(errs) != 1 || errs[0].Field != "title" {
		t.Fatalf("expected 1 error on field 'title', got: %v", errs)
	}

	// 2. field_cleared
	ruleCleared := Rule{
		ID: "test_cleared",
		Op: OpPredicateDSL,
		Config: map[string]any{
			"expression": "field_cleared:deprecated_field",
		},
	}
	errs = evalOverlayRule(nil, "backlog_item", map[string]any{}, nil, ruleCleared)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for unset deprecated_field, got: %v", errs)
	}
	errs = evalOverlayRule(nil, "backlog_item", map[string]any{"deprecated_field": "val"}, nil, ruleCleared)
	if len(errs) != 1 || errs[0].Field != "deprecated_field" {
		t.Fatalf("expected 1 error on field 'deprecated_field', got: %v", errs)
	}

	// 3. Status-scoped evaluation
	ruleStatusScoped := Rule{
		ID: "test_status_scoped",
		Op: OpPredicateDSL,
		Config: map[string]any{
			"expression": "field_nonempty:execution_field",
			"statuses":   []any{"in_progress"},
		},
	}
	// In draft status, rule is skipped
	errs = evalOverlayRule(nil, "backlog_item", map[string]any{"status": "draft"}, nil, ruleStatusScoped)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for draft status when rule gates in_progress, got: %v", errs)
	}
	// In in_progress status, rule enforces
	errs = evalOverlayRule(nil, "backlog_item", map[string]any{"status": "in_progress"}, nil, ruleStatusScoped)
	if len(errs) != 1 || errs[0].Field != "execution_field" {
		t.Fatalf("expected 1 error for in_progress status without execution_field, got: %v", errs)
	}

	// 4. Legacy prose compiled to DSL
	ruleProse := Rule{
		ID: "test_prose",
		Op: OpPredicateDSL,
		Config: map[string]any{
			"expression": "title, summary, and path are populated",
		},
	}
	errs = evalOverlayRule(nil, "doc_entry", map[string]any{
		"title":   "Doc Title",
		"summary": "Doc Summary",
		"path":    "docs/foo.md",
	}, nil, ruleProse)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for fully populated doc_entry, got: %v", errs)
	}
	errs = evalOverlayRule(nil, "doc_entry", map[string]any{
		"title": "Doc Title",
	}, nil, ruleProse)
	if len(errs) != 2 { // summary and path missing
		t.Fatalf("expected 2 errors for missing summary and path, got: %v", errs)
	}
}
