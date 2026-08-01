package mcp

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestHandleGraphTraversal(t *testing.T) {
	t.Setenv(zqkenv.ZqkGraphEnabledLegacy(), "false")
	t.Setenv(zqkenv.GraphEnabled(), "false")
	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, result any, err error)
	}{
		{
			name:    "missing start_node_id",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "valid traversal with defaults",
			args: map[string]any{
				"start_node_id": "ITEM-623",
			},
			wantErr: true, // Graph backend not enabled by default
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error when graph backend is not enabled")
				}
				resultMap, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("Expected map result, got %T", result)
				}
				if resultMap["start_node_id"] != "ITEM-623" {
					t.Errorf("Expected start_node_id=ITEM-623, got %v", resultMap["start_node_id"])
				}
				if _, ok := resultMap["error"]; !ok {
					t.Error("Expected 'error' key in result when graph backend is unavailable")
				}
			},
		},
		{
			name: "traversal with relationship and direction",
			args: map[string]any{
				"start_node_id": "GOAL-001",
				"relationship":  "HAS_CHILD",
				"direction":     "outgoing",
				"max_depth":     2,
			},
			wantErr: true, // Graph backend not enabled by default
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error when graph backend is not enabled")
				}
				resultMap, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("Expected map result, got %T", result)
				}
				if resultMap["relationship"] != "HAS_CHILD" {
					t.Errorf("Expected relationship=HAS_CHILD, got %v", resultMap["relationship"])
				}
				if resultMap["direction"] != "outgoing" {
					t.Errorf("Expected direction=outgoing, got %v", resultMap["direction"])
				}
				// max_depth should be 2 (accept int or float64)
				md := resultMap["max_depth"]
				switch v := md.(type) {
				case int:
					if v != 2 {
						t.Errorf("Expected max_depth=2, got %d", v)
					}
				case float64:
					if v != 2 {
						t.Errorf("Expected max_depth=2, got %v", v)
					}
				default:
					t.Errorf("Expected max_depth=2, got %v (type %T)", md, md)
				}
			},
		},
		{
			name: "traversal with filters",
			args: map[string]any{
				"start_node_id":     "MIL-001",
				"filter_labels":     []any{"BacklogItem", "Goal"},
				"filter_properties": map[string]any{objects.FieldKeyStatus: objects.ObjectStatusActive},
				"limit":             50,
			},
			wantErr: true, // Graph backend not enabled by default
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error when graph backend is not enabled")
				}
				resultMap, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("Expected map result, got %T", result)
				}
				if resultMap["start_node_id"] != "MIL-001" {
					t.Errorf("Expected start_node_id=MIL-001, got %v", resultMap["start_node_id"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HandleGraphTraversal(pkgctx.NewSystemContext(), tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleGraphTraversal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, result, err)
			}
		})
	}
}

func TestHandleResolveReferences(t *testing.T) {
	t.Setenv(zqkenv.ZqkGraphEnabledLegacy(), "false")
	t.Setenv(zqkenv.GraphEnabled(), "false")
	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, result any, err error)
	}{
		{
			name:    "missing references",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "empty references list",
			args: map[string]any{
				"references": []any{},
			},
			wantErr: true,
		},
		{
			name: "invalid references type",
			args: map[string]any{
				"references": "not-a-list",
			},
			wantErr: true,
		},
		{
			name: "valid references with kind:id format",
			args: map[string]any{
				"references": []any{"goal:GOAL-123", "milestone:MIL-456"},
			},
			wantErr: true, // Graph backend not enabled by default
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error when graph backend is not enabled")
				}
			},
		},
		{
			name: "references with id only (inferred kind)",
			args: map[string]any{
				"references": []any{"ITEM-623", "MIL-001"},
			},
			wantErr: true, // Graph backend not enabled by default
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error when graph backend is not enabled")
				}
			},
		},
		{
			name: "references with include_related",
			args: map[string]any{
				"references":           []any{"goal:GOAL-123"},
				"include_related":      true,
				objects.FieldKeyFormat: "yaml",
			},
			wantErr: true, // Graph backend not enabled by default
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error when graph backend is not enabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HandleResolveReferences(pkgctx.NewSystemContext(), tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleResolveReferences() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, result, err)
			}
		})
	}
}

func TestHandleStateAwareQuery(t *testing.T) {
	t.Setenv(zqkenv.ZqkGraphEnabledLegacy(), "false")
	t.Setenv(zqkenv.GraphEnabled(), "false")
	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, result any, err error)
	}{
		{
			name:    "missing query_type",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "invalid query_type",
			args: map[string]any{
				"query_type": "invalid_type",
			},
			wantErr: true, // Handler should return error for unknown query type
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error for invalid query_type")
				}
			},
		},
		{
			name: "active_items query",
			args: map[string]any{
				"query_type": "active_items",
			},
			wantErr: true, // Graph backend not enabled by default
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error when graph backend is not enabled")
				}
			},
		},
		{
			name: "blocked_items query",
			args: map[string]any{
				"query_type": "blocked_items",
			},
			wantErr: true, // Graph backend not enabled by default
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error when graph backend is not enabled")
				}
			},
		},
		{
			name: "query with filters and metrics",
			args: map[string]any{
				"query_type":           "dependencies",
				"filters":              map[string]any{objects.FieldKeyKind: "backlog_item"},
				"include_metrics":      true,
				objects.FieldKeyFormat: "markdown",
			},
			wantErr: true, // Graph backend not enabled by default
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Error("Expected error when graph backend is not enabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HandleStateAwareQuery(pkgctx.NewSystemContext(), tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleStateAwareQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, result, err)
			}
		})
	}
}

func TestParseReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ref      string
		wantKind string
		wantID   string
	}{
		{"goal:GOAL-123", "goal", "GOAL-123"},
		{"milestone:MIL-456", "milestone", "MIL-456"},
		{"ITEM-623", "", "ITEM-623"},
		{"", "", ""},
		{"invalid", "", "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			kind, id := parseReference(tt.ref)
			if kind != tt.wantKind {
				t.Errorf("parseReference(%q) kind = %q, want %q", tt.ref, kind, tt.wantKind)
			}
			if id != tt.wantID {
				t.Errorf("parseReference(%q) id = %q, want %q", tt.ref, id, tt.wantID)
			}
		})
	}
}

func TestInferKindFromID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id       string
		wantKind string
	}{
		{"ITEM-623", "backlog_item"},
		{"MIL-001", "milestone"},
		{"GOAL-123", "goal"},
		{"WS-001", "workstream"},
		{"PLAN-001", "priority_plan"},
		{"REQ-001", "requirement"},
		{"TEST-001", "test_case"},
		{"CRIT-001", "criteria"},
		{"DEC-001", "decision"},
		{"ROAD-001", "roadmap"},
		{"UNKNOWN-001", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			kind := inferKindFromID(tt.id)
			if kind != tt.wantKind {
				t.Errorf("inferKindFromID(%q) = %q, want %q", tt.id, kind, tt.wantKind)
			}
		})
	}
}
