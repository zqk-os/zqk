package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestResolveBaseAnchor(t *testing.T) {
	objectsMap := map[string]map[string]any{
		"PRI-1": {objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: "planned"},
		"HOT-1": {objects.FieldKeyKind: "hotfix"},
		"TSK-1": {objects.FieldKeyKind: "task", objects.FieldKeyBacklogItemRef: "BLI-1"},
		"BLI-1": {objects.FieldKeyKind: "backlog_item", objects.FieldKeyWorkstreamRef: "WRK-1"},
		"WRK-1": {objects.FieldKeyKind: "workstream", objects.FieldKeyPriorityPlanRef: "PRI-2"},
		"PRI-2": {objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: "active"},
		"TSK-2": {objects.FieldKeyKind: "task", objects.FieldKeyBacklogItemRef: "BLI-2"},
		"BLI-2": {objects.FieldKeyKind: "backlog_item", objects.FieldKeyPriorityPlanRef: "PRI-3"},
		"PRI-3": {objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: "in_progress"},
		"TSK-3": {objects.FieldKeyKind: "task", objects.FieldKeyBacklogItemRef: "BLI-3"},
		"BLI-3": {objects.FieldKeyKind: "backlog_item", objects.FieldKeyPriorityPlanRef: "PRI-4"},
		"PRI-4": {objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: "archived"},
		"TSK-4": {objects.FieldKeyKind: "task"},
	}

	//nolint:unparam
	reader := func(ctx context.Context, id string) (map[string]any, error) {
		if obj, ok := objectsMap[id]; ok {
			return obj, nil
		}
		return nil, fmt.Errorf("not found")
	}

	tests := []struct {
		name     string
		startID  string
		expected string
	}{
		{
			name:     "direct priority plan",
			startID:  "PRI-1",
			expected: "integration/pri-1",
		},
		{
			name:     "hotfix traces to main",
			startID:  "HOT-1",
			expected: "main",
		},
		{
			name:     "task to priority plan",
			startID:  "TSK-1",
			expected: "integration/pri-2",
		},
		{
			name:     "task to priority plan via backlog_item.priority_plan_ref directly",
			startID:  "TSK-2",
			expected: "integration/pri-3",
		},
		{
			name:     "archived priority plan defaults to main",
			startID:  "TSK-3",
			expected: "main",
		},
		{
			name:     "no upward references defaults to main",
			startID:  "TSK-4",
			expected: "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveBaseAnchor(context.Background(), reader, tt.startID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.expected {
				t.Errorf("resolveBaseAnchor(%q) = %q, want %q", tt.startID, got, tt.expected)
			}
		})
	}
}
