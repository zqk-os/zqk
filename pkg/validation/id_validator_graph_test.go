package validation_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
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
			ID:     validation.ConstMagic55c9d1b2,
			Labels: []string{"ObjectSpec"},
			Properties: map[string]any{
				objects.FieldKeyOntology: "backlog_item",
				"id_template":            "BLI-{sequence}",
			},
		},
		{
			ID:     validation.ConstMagic7a7c4469,
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
		t.Fatalf(validation.ConstMagic29dfaddc, err)
	}

	// Test backlog_item
	valid, err := validator.ValidateID("BLI-001", "backlog_item")
	if err != nil {
		t.Errorf(validation.ConstMagicc3a9031f, err)
	}
	if !valid {
		t.Error(validation.ConstMagicae27b884)
	}

	// Test priority_plan with both prefixes
	valid, err = validator.ValidateID("PRI-208", "priority_plan")
	if err != nil {
		t.Errorf(validation.ConstMagicc3a9031f, err)
	}
	if !valid {
		t.Error(validation.ConstMagic214c3057)
	}

	valid, err = validator.ValidateID("PRIO-002", "priority_plan")
	if err != nil {
		t.Errorf(validation.ConstMagicc3a9031f, err)
	}
	if !valid {
		t.Error(validation.ConstMagic59f32d4b)
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
		t.Fatalf(validation.ConstMagic34fb2cbb, err)
	}

	// Should still have defaults loaded
	//nolint:errcheck // Test helper - error acceptable
	valid, _ := validator.ValidateID("BLI-001", "backlog_item")
	if !valid {
		t.Error(validation.ConstMagicb7163a28)
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
			name: validation.ConstMagica9eb8343,
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "backlog_item",
				},
			},
			wantKind:    "backlog_item",
			wantNil:     false,
			description: validation.ConstMagicdd568673,
		},
		{
			name: validation.ConstMagic6a353d45,
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyKind: "milestone",
				},
			},
			wantKind:    "milestone",
			wantNil:     false,
			description: validation.ConstMagic1db538b9,
		},
		{
			name: validation.ConstMagic91a766a7,
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyType: "goal",
				},
			},
			wantKind:    "goal",
			wantNil:     false,
			description: validation.ConstMagice5f3c624,
		},
		{
			name: validation.ConstMagiceacaf5dd,
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "priority_plan",
					"id_template":            "PRI-{sequence}",
				},
			},
			wantKind:    "priority_plan",
			wantNil:     false,
			description: validation.ConstMagicf5c12fa3,
		},
		{
			name: validation.ConstMagicc9269618,
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "requirement",
					"id_prefixes":            []any{"REQ-", "REQU-"},
				},
			},
			wantKind:    "requirement",
			wantNil:     false,
			description: validation.ConstMagic72dbbaf5,
		},
		{
			name: validation.ConstMagicee8e2026,
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
			description: validation.ConstMagic68c9a719,
		},
		{
			name: validation.ConstMagica04884e5,
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "",
				},
			},
			wantKind:    "",
			wantNil:     true,
			description: validation.ConstMagiccbe38769,
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
			name: validation.ConstMagic3f4752e5,
			node: &provider.Node{
				Properties: map[string]any{},
			},
			wantKind:    "",
			wantNil:     true,
			description: validation.ConstMagic08c60601,
		},
		{
			name: validation.ConstMagic5b08df7d,
			node: &provider.Node{
				Properties: map[string]any{
					objects.FieldKeyOntology: "test_kind",
					"id_prefixes":            []any{"PREFIX-", 123, "OTHER-"},
				},
			},
			wantKind:    "test_kind",
			wantNil:     false,
			description: validation.ConstMagic0e48b4bd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validation.ParseGraphSpecNodeForTest(validator, tt.node)
			if tt.wantNil {
				if got != nil {
					t.Errorf(validation.ConstMagic315f7ce3, got, tt.description)
				}
				return
			}
			if got == nil {
				t.Fatalf(validation.ConstMagice9320605, tt.description)
			}
			if got.Kind != tt.wantKind {
				t.Errorf(validation.ConstMagic45bcc88b, got.Kind, tt.wantKind, tt.description)
			}
		})
	}
}
