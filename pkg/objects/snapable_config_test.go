package objects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestGetSnapableObjectQuery(t *testing.T) {
	t.Parallel()
	// Create a temporary traits directory with snapable trait
	tmpDir := t.TempDir()
	traitsDir := filepath.Join(tmpDir, "traits")
	if err := os.MkdirAll(traitsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create traits directory: %v", err)
	}

	// Write snapable trait with object_config
	snapableYAML := `name: snapable
description: Trait for snapshot operations
category: system
object_level: true
field_level: true
requires:
    - readable
conflicts: []
includes: []
version: 2.0.0
status: active
object_config:
    default_query:
        filters:
            status: {"$in": ["active", "in_progress"]}
        sort_by: "created_at"
        sort_asc: false
        limit: 100
    query_presets:
        active_only:
            filters:
                status: "active"
        recent:
            filters:
                created_at: {"$gte": "2026-01-01T00:00:00Z"}
            sort_by: "created_at"
            sort_asc: false
            limit: 50
field_config:
    data_handlers:
        include:
            handler: "include"
        exclude_pii:
            handler: "exclude"
            condition:
                field_tags: {"$has": "pii"}
`

	snapablePath := filepath.Join(traitsDir, "snapable.yaml")
	if err := os.WriteFile(snapablePath, []byte(snapableYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write snapable trait: %v", err)
	}

	// Create trait registry and load from directory
	registry := &TraitRegistry{
		traits:       make(map[string]*TraitDefinition),
		conflicts:    make(map[string][]string),
		dependencies: make(map[string][]string),
	}
	// Load traits from our temp directory
	if err := registry.LoadTraitsFromDirectory(traitsDir); err != nil {
		t.Fatalf("Failed to load traits: %v", err)
	}

	// Test GetSnapableObjectQuery
	objQuery, err := GetSnapableObjectQuery(registry)
	if err != nil {
		t.Fatalf("GetSnapableObjectQuery failed: %v", err)
	}

	if objQuery == nil {
		t.Fatal("GetSnapableObjectQuery returned nil")
	}

	// Verify default_query
	if objQuery.DefaultQuery == nil {
		t.Fatal("DefaultQuery is nil")
	}

	// Check filters
	filters, ok := objQuery.DefaultQuery["filters"].(map[string]any)
	if !ok {
		t.Fatal("DefaultQuery filters is not a map")
	}

	statusFilter, ok := filters[FieldKeyStatus].(map[string]any)
	if !ok {
		t.Fatal("status filter is not a map")
	}

	inValues, ok := statusFilter["$in"].([]any)
	if !ok {
		t.Fatal("$in operator value is not a slice")
	}

	if len(inValues) != 2 {
		t.Fatalf("Expected 2 values in $in, got %d", len(inValues))
	}

	// Check sort_by
	sortBy, ok := objQuery.DefaultQuery["sort_by"].(string)
	if !ok || sortBy != "created_at" {
		t.Fatalf("Expected sort_by='created_at', got %v", sortBy)
	}

	// Check sort_asc
	sortAsc, ok := objQuery.DefaultQuery["sort_asc"].(bool)
	if !ok || sortAsc != false {
		t.Fatalf("Expected sort_asc=false, got %v", sortAsc)
	}

	// Check limit
	limit, ok := objQuery.DefaultQuery["limit"].(int)
	if !ok || limit != 100 {
		t.Fatalf("Expected limit=100, got %v", limit)
	}

	// Verify query_presets
	if len(objQuery.QueryPresets) != 2 {
		t.Fatalf("Expected 2 query presets, got %d", len(objQuery.QueryPresets))
	}

	// Check active_only preset
	activeOnly, ok := objQuery.QueryPresets["active_only"]
	if !ok {
		t.Fatal("active_only preset not found")
	}

	activeFilters, ok := activeOnly["filters"].(map[string]any)
	if !ok {
		t.Fatal("active_only filters is not a map")
	}

	if activeFilters[FieldKeyStatus] != "active" {
		t.Fatalf("Expected status='active', got %v", activeFilters[FieldKeyStatus])
	}

	// Check recent preset
	recent, ok := objQuery.QueryPresets["recent"]
	if !ok {
		t.Fatal("recent preset not found")
	}

	recentFilters, ok := recent["filters"].(map[string]any)
	if !ok {
		t.Fatal("recent filters is not a map")
	}

	createdAtFilter, ok := recentFilters[FieldKeyCreatedAt].(map[string]any)
	if !ok {
		t.Fatal("created_at filter is not a map")
	}

	if createdAtFilter["$gte"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("Expected $gte='2026-01-01T00:00:00Z', got %v", createdAtFilter["$gte"])
	}
}

func TestGetSnapableFieldHandlers(t *testing.T) {
	t.Parallel()
	// Test with field definition that has snapable configuration
	fieldDef := map[string]any{
		FieldKeyType: "string",
		"traits":     []any{"readable", "snapable"},
		"snapable": map[string]any{
			"field_config": map[string]any{
				"data_handlers": map[string]any{
					"default": map[string]any{
						"handler": "include",
					},
					"exclude_pii": map[string]any{
						"handler": "exclude",
						"condition": map[string]any{
							"field_tags": map[string]any{
								"$has": "pii",
							},
						},
					},
					"redact_email": map[string]any{
						"handler": "redact",
						"params": map[string]any{
							"pattern":     "email",
							"replacement": "***@***.***",
						},
					},
				},
			},
		},
	}

	fieldConfig, err := GetSnapableFieldHandlers(fieldDef)
	if err != nil {
		t.Fatalf("GetSnapableFieldHandlers failed: %v", err)
	}

	if fieldConfig == nil {
		t.Fatal("GetSnapableFieldHandlers returned nil")
	}

	if len(fieldConfig.DataHandlers) != 3 {
		t.Fatalf("Expected 3 handlers, got %d", len(fieldConfig.DataHandlers))
	}

	// Check default handler
	defaultHandler, ok := fieldConfig.DataHandlers["default"]
	if !ok {
		t.Fatal("default handler not found")
	}
	if defaultHandler.Handler != "include" {
		t.Fatalf("Expected handler='include', got %s", defaultHandler.Handler)
	}

	// Check exclude_pii handler
	excludeHandler, ok := fieldConfig.DataHandlers["exclude_pii"]
	if !ok {
		t.Fatal("exclude_pii handler not found")
	}
	if excludeHandler.Handler != "exclude" {
		t.Fatalf("Expected handler='exclude', got %s", excludeHandler.Handler)
	}
	if excludeHandler.Condition == nil {
		t.Fatal("exclude_pii handler missing condition")
	}

	// Check redact_email handler
	redactHandler, ok := fieldConfig.DataHandlers["redact_email"]
	if !ok {
		t.Fatal("redact_email handler not found")
	}
	if redactHandler.Handler != "redact" {
		t.Fatalf("Expected handler='redact', got %s", redactHandler.Handler)
	}
	if redactHandler.Params == nil {
		t.Fatal("redact_email handler missing params")
	}
	if redactHandler.Params["pattern"] != "email" {
		t.Fatalf("Expected pattern='email', got %v", redactHandler.Params["pattern"])
	}
}

func TestGetSnapableFieldHandlers_NoConfig(t *testing.T) {
	t.Parallel()
	// Test with field definition without snapable configuration
	fieldDef := map[string]any{
		FieldKeyType: "string",
		"traits":     []any{"readable"},
	}

	fieldConfig, err := GetSnapableFieldHandlers(fieldDef)
	if err != nil {
		t.Fatalf("GetSnapableFieldHandlers failed: %v", err)
	}

	if fieldConfig != nil {
		t.Fatal("Expected nil for field without snapable config")
	}
}

func TestApplyFieldHandler(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   any
		handler *SnapableFieldHandler
		want    any
		wantErr bool
	}{
		{
			name:  "include handler",
			value: "test@example.com",
			handler: &SnapableFieldHandler{
				Handler: "include",
			},
			want:    "test@example.com",
			wantErr: false,
		},
		{
			name:  "exclude handler",
			value: "test@example.com",
			handler: &SnapableFieldHandler{
				Handler: "exclude",
			},
			want:    nil, // Signal to exclude
			wantErr: false,
		},
		{
			name:  "null handler",
			value: "test@example.com",
			handler: &SnapableFieldHandler{
				Handler: "null",
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:  "hash handler",
			value: "sensitive-data",
			handler: &SnapableFieldHandler{
				Handler: "hash",
				Params: map[string]any{
					FieldKeyAlgorithm: "sha256",
				},
			},
			want:    "[sha256_hash]", // Placeholder for now
			wantErr: false,
		},
		{
			name:  "redact handler",
			value: "test@example.com",
			handler: &SnapableFieldHandler{
				Handler: "redact",
				Params: map[string]any{
					"replacement": "***@***.***",
				},
			},
			want:    "***@***.***",
			wantErr: false,
		},
		{
			name:  "unknown handler",
			value: "test",
			handler: &SnapableFieldHandler{
				Handler: "unknown",
			},
			want:    "test", // Returns as-is
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldDef := map[string]any{}
			got, err := ApplyFieldHandler(tt.value, tt.handler, fieldDef)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyFieldHandler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ApplyFieldHandler() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyFieldHandler_WithCondition(t *testing.T) {
	t.Parallel()
	// Test handler with condition that matches
	fieldDef := map[string]any{
		"field_tags": []any{"pii", "sensitive"},
	}

	handler := &SnapableFieldHandler{
		Handler: "exclude",
		Condition: map[string]any{
			"field_tags": map[string]any{
				"$has": "pii",
			},
		},
	}

	value := "test@example.com"
	result, err := ApplyFieldHandler(value, handler, fieldDef)
	if err != nil {
		t.Fatalf("ApplyFieldHandler failed: %v", err)
	}

	if result != nil {
		t.Fatal("Expected field to be excluded when condition matches")
	}

	// Test handler with condition that doesn't match
	fieldDefNoPII := map[string]any{
		"field_tags": []any{"public"},
	}

	result2, err := ApplyFieldHandler(value, handler, fieldDefNoPII)
	if err != nil {
		t.Fatalf("ApplyFieldHandler failed: %v", err)
	}

	if result2 != value {
		t.Fatalf("Expected field to be included when condition doesn't match, got %v", result2)
	}
}

func TestEvaluateCondition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		condition map[string]any
		fieldDef  map[string]any
		want      bool
	}{
		{
			name: "field_tags $has matches",
			condition: map[string]any{
				"field_tags": map[string]any{
					"$has": "pii",
				},
			},
			fieldDef: map[string]any{
				"field_tags": []any{"pii", "sensitive"},
			},
			want: true,
		},
		{
			name: "field_tags $has doesn't match",
			condition: map[string]any{
				"field_tags": map[string]any{
					"$has": "pii",
				},
			},
			fieldDef: map[string]any{
				"field_tags": []any{"public"},
			},
			want: false,
		},
		{
			name: "security matches",
			condition: map[string]any{
				"security": "sensitive",
			},
			fieldDef: map[string]any{
				"security": "sensitive",
			},
			want: true,
		},
		{
			name: "security doesn't match",
			condition: map[string]any{
				"security": "sensitive",
			},
			fieldDef: map[string]any{
				"security": "non-sensitive",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateCondition(tt.condition, tt.fieldDef)
			if got != tt.want {
				t.Errorf("evaluateCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}
