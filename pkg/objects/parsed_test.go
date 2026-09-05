package objects

import (
	"testing"
	"time"
)

func TestParseObject(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyID:            "BLI-001",
		FieldKeyKind:          "backlog_item",
		FieldKeyTitle:         "Test Item",
		FieldKeyStatus:        "exploring",
		FieldKeyCategory:      "Product",
		FieldKeyPriority:      "high",
		FieldKeySchemaVersion: DefaultSchemaVersion,
		FieldKeyCreatedAt:     "2025-01-01T00:00:00Z",
		FieldKeyUpdatedAt:     "2025-01-02T00:00:00Z",
		FieldKeyGoalRefs:      []any{"goal:GOAL-001", "goal:GOAL-002"},
		FieldKeyMilestoneRefs: []string{"milestone:MIL-001"},
		FieldKeyStatusHistory: []any{
			map[string]any{
				FieldKeyStatus: "exploring",
				"timestamp":    "2025-01-01T00:00:00Z",
				FieldKeyReason: "Initial creation",
			},
		},
		FieldKeyChangeLog: []any{
			map[string]any{
				"timestamp":  "2025-01-02T00:00:00Z",
				"changed_by": "account:user",
				"changes": map[string]any{
					FieldKeyTitle: map[string]any{
						"old": "Old Title",
						"new": "New Title",
					},
				},
			},
		},
	}

	parsed, err := ParseObject(obj)
	if err != nil {
		t.Fatalf("ParseObject failed: %v", err)
	}

	// Verify extracted fields
	if parsed.ID != "BLI-001" {
		t.Errorf("expected ID 'BLI-001', got %s", parsed.ID)
	}
	if parsed.Kind != "backlog_item" {
		t.Errorf("expected kind 'backlog_item', got %s", parsed.Kind)
	}
	if parsed.Status != "exploring" {
		t.Errorf("expected status 'exploring', got %s", parsed.Status)
	}

	// Verify list fields were extracted
	if len(parsed.GoalRefs) != 2 {
		t.Errorf("expected 2 goal_refs, got %d", len(parsed.GoalRefs))
	}
	if parsed.GoalRefs[0] != "goal:GOAL-001" {
		t.Errorf("expected first goal_ref 'goal:GOAL-001', got %s", parsed.GoalRefs[0])
	}

	if len(parsed.MilestoneRefs) != 1 {
		t.Errorf("expected 1 milestone_ref, got %d", len(parsed.MilestoneRefs))
	}

	// Verify complex structures were extracted
	if len(parsed.StatusHistory) != 1 {
		t.Errorf("expected 1 status_history entry, got %d", len(parsed.StatusHistory))
	}
	if parsed.StatusHistory[0].Status != "exploring" {
		t.Errorf("expected status 'exploring', got %s", parsed.StatusHistory[0].Status)
	}

	if len(parsed.ChangeLog) != 1 {
		t.Errorf("expected 1 change_log entry, got %d", len(parsed.ChangeLog))
	}

	// Verify extracted fields were removed from raw map
	if _, ok := obj[FieldKeyGoalRefs]; ok {
		t.Error("goal_refs should have been removed from raw map")
	}
	if _, ok := obj[FieldKeyStatusHistory]; ok {
		t.Error("status_history should have been removed from raw map")
	}
	if _, ok := obj[FieldKeyChangeLog]; ok {
		t.Error("change_log should have been removed from raw map")
	}

	// Verify simple fields remain in raw map
	if _, ok := obj[FieldKeyID]; !ok {
		t.Error("id should remain in raw map")
	}
	if _, ok := obj[FieldKeyTitle]; !ok {
		t.Error("title should remain in raw map")
	}
}

func TestParseObject_ToMap(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyID:       "BLI-001",
		FieldKeyTitle:    "Test Item",
		FieldKeyGoalRefs: []any{"goal:GOAL-001"},
		FieldKeyStatusHistory: []any{
			map[string]any{
				FieldKeyStatus: "exploring",
				"timestamp":    "2025-01-01T00:00:00Z",
			},
		},
	}

	parsed, err := ParseObject(obj)
	if err != nil {
		t.Fatalf("ParseObject failed: %v", err)
	}

	// Convert back to map
	result := parsed.ToMap()

	// Verify fields were restored
	if result[FieldKeyID] != "BLI-001" {
		t.Errorf("expected id 'BLI-001', got %v", result[FieldKeyID])
	}

	if goalRefs, ok := result[FieldKeyGoalRefs].([]string); ok {
		if len(goalRefs) != 1 {
			t.Errorf("expected 1 goal_ref, got %d", len(goalRefs))
		}
	} else {
		t.Error("goal_refs should be restored as []string")
	}

	if _, ok := result[FieldKeyStatusHistory]; !ok {
		t.Error("status_history should be restored")
	}
}

func TestParseObject_GetField(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyID:              "BLI-001",
		FieldKeyKind:            "backlog_item",
		FieldKeyStatus:          "exploring",
		FieldKeyTitle:           "Test Item",
		FieldKeyCategory:        "Product",
		FieldKeyPriority:        "high",
		FieldKeySchemaVersion:   DefaultSchemaVersion,
		FieldKeyCreatedAt:       "2025-01-01T00:00:00Z",
		FieldKeyUpdatedAt:       "2025-01-02T00:00:00Z",
		FieldKeyGoalRefs:        []any{"goal:GOAL-001"},
		FieldKeyMilestoneRefs:   []any{"milestone:MIL-001"},
		FieldKeyRequirementRefs: []any{"requirement:REQ-001"},
		FieldKeyWorkstreamRefs:  []any{"workstream:WS-001"},
		FieldKeyBacklogItemRefs: []any{"backlog_item:BLI-002"},
		FieldKeyStatusHistory: []any{
			map[string]any{
				FieldKeyStatus: "exploring",
				"timestamp":    "2025-01-01T00:00:00Z",
			},
		},
		FieldKeyChangeLog: []any{
			map[string]any{
				"timestamp": "2025-01-02T00:00:00Z",
			},
		},
		"custom_field": "custom_value",
	}

	parsed, err := ParseObject(obj)
	if err != nil {
		t.Fatalf("ParseObject failed: %v", err)
	}

	// Test all typed field access
	tests := []struct {
		name     string
		expected any
	}{
		{"id", "BLI-001"},
		{"kind", "backlog_item"},
		{"status", "exploring"},
		{"title", "Test Item"},
		{"category", "Product"},
		{"priority", "high"},
		{"schema_version", DefaultSchemaVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := parsed.GetField(tt.name)
			if !ok {
				t.Errorf("expected %s to be found", tt.name)
			}
			if val != tt.expected {
				t.Errorf("expected %s %v, got %v", tt.name, tt.expected, val)
			}
		})
	}

	// Test time fields
	if val, ok := parsed.GetField("created_at"); !ok {
		t.Error("expected created_at to be found")
	} else if _, ok := val.(time.Time); !ok {
		t.Errorf("expected created_at to be time.Time, got %T", val)
	}

	if val, ok := parsed.GetField("updated_at"); !ok {
		t.Error("expected updated_at to be found")
	} else if _, ok := val.(time.Time); !ok {
		t.Errorf("expected updated_at to be time.Time, got %T", val)
	}

	// Test list field access
	listTests := []struct {
		name     string
		expected int
	}{
		{FieldKeyGoalRefs, 1},
		{FieldKeyMilestoneRefs, 1},
		{FieldKeyRequirementRefs, 1},
		{FieldKeyWorkstreamRefs, 1},
		{FieldKeyBacklogItemRefs, 1},
		{"status_history", 1},
		{"change_log", 1},
	}

	for _, tt := range listTests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := parsed.GetField(tt.name)
			if !ok {
				t.Errorf("expected %s to be found", tt.name)
			} else {
				switch v := val.(type) {
				case []string:
					if len(v) != tt.expected {
						t.Errorf("expected %s to have %d items, got %d", tt.name, tt.expected, len(v))
					}
				case []StatusHistoryEntry:
					if len(v) != tt.expected {
						t.Errorf("expected %s to have %d items, got %d", tt.name, tt.expected, len(v))
					}
				case []ChangeLogEntry:
					if len(v) != tt.expected {
						t.Errorf("expected %s to have %d items, got %d", tt.name, tt.expected, len(v))
					}
				default:
					t.Errorf("unexpected type for %s: %T", tt.name, val)
				}
			}
		})
	}

	// Test raw map field access (fallback)
	if val, ok := parsed.GetField("custom_field"); !ok || val != "custom_value" {
		t.Errorf("expected custom_field 'custom_value', got %v", val)
	}

	// Test non-existent field
	if _, ok := parsed.GetField("nonexistent"); ok {
		t.Error("expected nonexistent field to return false")
	}

	// Test empty typed fields fall back to raw map
	emptyObj := map[string]any{
		FieldKeyID:    "",
		FieldKeyTitle: "Raw Title",
	}
	emptyParsed, err := ParseObject(emptyObj)
	if err != nil {
		t.Fatalf("ParseObject failed: %v", err)
	}

	// Empty ID should fall back to raw map
	if val, ok := emptyParsed.GetField("id"); !ok || val != emptyValue {
		t.Errorf("expected empty id from raw map, got %v", val)
	}

	// Title should come from raw map
	if val, ok := emptyParsed.GetField("title"); !ok || val != "Raw Title" {
		t.Errorf("expected title 'Raw Title', got %v", val)
	}
}

func TestExtractStringList(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		"refs": []any{"ref1", "ref2", "ref3"},
	}

	result := extractStringList(obj, "refs")
	if len(result) != 3 {
		t.Errorf("expected 3 refs, got %d", len(result))
	}
	if result[0] != "ref1" {
		t.Errorf("expected first ref 'ref1', got %s", result[0])
	}

	// Verify it was removed from map
	if _, ok := obj["refs"]; ok {
		t.Error("refs should have been removed from map")
	}
}

func TestExtractStringList_AlreadyTyped(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		"refs": []string{"ref1", "ref2"},
	}

	result := extractStringList(obj, "refs")
	if len(result) != 2 {
		t.Errorf("expected 2 refs, got %d", len(result))
	}

	// Verify it was removed from map
	if _, ok := obj["refs"]; ok {
		t.Error("refs should have been removed from map")
	}
}

func TestExtractStringList_Empty(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		"other_field": "value",
	}

	result := extractStringList(obj, "nonexistent")
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result))
	}

	// Verify other field wasn't touched
	if obj["other_field"] != "value" {
		t.Error("other_field should not have been modified")
	}
}
