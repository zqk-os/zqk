package validation_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestIDValidator_LoadPatternsFromGraph(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	ctx := context.Background()
	conn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}
	defer func() { _ = pool.ReturnConnection(conn) }()

	nodes := []provider.Node{
		{
			ID:     "spec_backlog_item",
			Labels: []string{"ObjectSpec"},
			Properties: map[string]any{
				objects.FieldKeyOntology: "backlog_item",
				"id_template":            "BLI-{sequence}",
			},
		},
		{
			ID:     "spec_priority_plan",
			Labels: []string{"ObjectSpec"},
			Properties: map[string]any{
				objects.FieldKeyOntology: "priority_plan",
				"id_prefixes":            []any{"PRI-", "PRIO-"},
			},
		},
	}

	for _, n := range nodes {
		_ = conn.CreateNode(ctx, n)
	}
	defer func() {
		for _, n := range nodes {
			_ = conn.DeleteNode(ctx, n.ID, n.Labels)
		}
	}()

	validator := validation.NewIDValidatorWithGraph("", conn)
	err = validator.LoadPatterns()
	if err != nil {
		t.Fatalf("LoadPatterns() error = %v", err)
	}

	// Test backlog_item
	valid, err := validator.ValidateID("BLI-001", "backlog_item")
	if err != nil {
		t.Errorf("ValidateID() error = %v", err)
	}
	if !valid {
		t.Error("BLI-001 should be valid for backlog_item")
	}

	// Test priority_plan with both prefixes
	valid, err = validator.ValidateID("PRI-208", "priority_plan")
	if err != nil {
		t.Errorf("ValidateID() error = %v", err)
	}
	if !valid {
		t.Error("PRI-208 should be valid for priority_plan")
	}

	valid, err = validator.ValidateID("PRIO-002", "priority_plan")
	if err != nil {
		t.Errorf("ValidateID() error = %v", err)
	}
	if !valid {
		t.Error("PRIO-002 should be valid for priority_plan")
	}
}

func TestIDValidator_GraphFallbackToFiles(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	ctx := context.Background()
	conn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}
	defer func() { _ = pool.ReturnConnection(conn) }()

	validator := validation.NewIDValidatorWithGraph("", conn)
	// If graph returns empty, should fall back to file-based or defaults
	err = validator.LoadPatterns()
	if err != nil {
		t.Fatalf("LoadPatterns() should not error on fallback: %v", err)
	}

	// Should still have defaults loaded
	//nolint:errcheck // Test helper - error acceptable
	valid, _ := validator.ValidateID("BLI-001", "backlog_item")
	if !valid {
		t.Error("Should have default patterns even if graph is empty")
	}
}

// TestParseGraphSpecNode tests the parseGraphSpecNode function
func TestParseGraphSpecNode(t *testing.T) {
	t.Parallel()
	validator := validation.NewIDValidator("")

	tests := []struct {
		name        string
		node        *provider.Node
		wantKind    string
		wantNil     bool
		description string
	}{
		{
			name: "node with ontology property",
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "backlog_item",
				},
			},
			wantKind:    "backlog_item",
			wantNil:     false,
			description: "Node with ontology property",
		},
		{
			name: "node with kind property (fallback)",
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyKind: "milestone",
				},
			},
			wantKind:    "milestone",
			wantNil:     false,
			description: "Node with kind property (ontology fallback)",
		},
		{
			name: "node with type property (fallback)",
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyType: "goal",
				},
			},
			wantKind:    "goal",
			wantNil:     false,
			description: "Node with type property (kind fallback)",
		},
		{
			name: "node with id_template",
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "priority_plan",
					"id_template":            "PRI-{sequence}",
				},
			},
			wantKind:    "priority_plan",
			wantNil:     false,
			description: "Node with id_template extracts prefix",
		},
		{
			name: "node with id_prefixes",
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "requirement",
					"id_prefixes":            []any{"REQ-", "REQU-"},
				},
			},
			wantKind:    "requirement",
			wantNil:     false,
			description: "Node with id_prefixes array",
		},
		{
			name: "node with nested pattern",
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "test_case",
					"fields": map[string]any{
						objects.FieldKeyID: map[string]any{
							"validation": map[string]any{
								"pattern": "^TEST-\\d{3,}$",
							},
						},
					},
				},
			},
			wantKind:    "test_case",
			wantNil:     false,
			description: "Node with nested pattern in fields.id.validation.pattern",
		},
		{
			name: "node with empty ontology",
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "",
				},
			},
			wantKind:    "",
			wantNil:     true,
			description: "Node with empty ontology returns nil",
		},
		{
			name: "node without ontology/kind/type",
			node: &provider.Node{
				Properties: map[string]any{
					"other": "value",
				},
			},
			wantKind:    "",
			wantNil:     true,
			description: "Node without ontology/kind/type returns nil",
		},
		{
			name: "node with empty properties",
			node: &provider.Node{
				Properties: map[string]any{},
			},
			wantKind:    "",
			wantNil:     true,
			description: "Node with empty properties returns nil",
		},
		{
			name: "node with id_prefixes containing non-strings",
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "test_kind",
					"id_prefixes":            []any{"PREFIX-", 123, "OTHER-"},
				},
			},
			wantKind:    "test_kind",
			wantNil:     false,
			description: "Node with id_prefixes filters out non-strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validation.ParseGraphSpecNodeForTest(validator, tt.node)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseGraphSpecNode() = %+v, want nil (%s)", got, tt.description)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseGraphSpecNode() returned nil, want non-nil (%s)", tt.description)
			}
			if got.Kind != tt.wantKind {
				t.Errorf("parseGraphSpecNode().Kind = %q, want %q (%s)", got.Kind, tt.wantKind, tt.description)
			}
		})
	}
}
