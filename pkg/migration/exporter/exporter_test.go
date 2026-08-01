package exporter

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestExportToYAML(t *testing.T) {
	t.Parallel()
	exporter := &GraphExporter{}

	properties := map[string]any{
		objects.FieldKeyID:     "ITEM-001",
		objects.FieldKeyKind:   "backlog_item",
		objects.FieldKeyTitle:  "Test Item",
		objects.FieldKeyStatus: "planned",
	}

	yamlData, err := exporter.ExportToYAML(properties)
	if err != nil {
		t.Fatalf("Failed to export to YAML: %v", err)
	}

	if len(yamlData) == 0 {
		t.Error("Expected YAML data, got empty")
	}

	// Verify YAML contains expected fields
	yamlStr := string(yamlData)
	if !strings.Contains(yamlStr, "id: ITEM-001") {
		t.Error("YAML should contain id field")
	}
	if !strings.Contains(yamlStr, "kind: backlog_item") {
		t.Error("YAML should contain kind field")
	}
}

func TestToLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind   string
		expect string
	}{
		{"backlog_item", "BacklogItem"},
		{"priority_plan", "PriorityPlan"},
		{"test_case", "TestCase"},
		{"goal", "Goal"},
	}

	for _, tt := range tests {
		result := toLabel(tt.kind)
		if result != tt.expect {
			t.Errorf("toLabel(%q) = %q, want %q", tt.kind, result, tt.expect)
		}
	}
}
