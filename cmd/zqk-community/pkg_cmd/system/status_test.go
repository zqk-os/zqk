package system

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestNewStatusCmd(t *testing.T) {
	t.Parallel()
	cmd := NewStatusCmd()
	if cmd == nil {
		t.Fatal("NewStatusCmd() returned nil")
	}

	if cmd.Use != "status" {
		t.Errorf("Expected command use to be 'status', got '%s'", cmd.Use)
	}

	if cmd.Short == emptyValue {
		t.Error("Command should have a short description")
	}
}

func TestStatusCommandFlags(t *testing.T) {
	t.Parallel()
	cmd := NewStatusCmd()

	// Test verbose flag
	if cmd.Flags().Lookup("verbose") == nil {
		t.Error("Command should have --verbose flag")
	}
}

func TestFormatStatusTablePlan_WithBacklogItems(t *testing.T) {
	t.Parallel()
	// Plan with backlog_items (ITEM-721 enhancement)
	statusData := map[string]any{
		"current_priority_plan": map[string]any{
			objects.FieldKeyID:    "PLAN-219",
			objects.FieldKeyTitle: "Process & System",
			"backlog_items":       29,
		},
	}
	out := formatStatusTablePlan(statusData)
	if out == emptyValue {
		t.Fatal("formatStatusTablePlan returned empty")
	}
	if !strings.Contains(out, "PLAN-219") || !strings.Contains(out, "Process & System") {
		t.Errorf("expected plan id and title in output, got %q", out)
	}
	if !strings.Contains(out, "29 backlog items") {
		t.Errorf("expected backlog count in output, got %q", out)
	}
}

func TestFormatStatusTablePlan_WithoutBacklogItems(t *testing.T) {
	t.Parallel()
	statusData := map[string]any{
		"current_priority_plan": map[string]any{
			objects.FieldKeyID:    "PLAN-001",
			objects.FieldKeyTitle: "Test Plan",
		},
	}
	out := formatStatusTablePlan(statusData)
	if out == emptyValue {
		t.Fatal("formatStatusTablePlan returned empty")
	}
	if strings.Contains(out, "backlog items") {
		t.Error("should not contain backlog items when not set")
	}
	if !strings.Contains(out, "PLAN-001") || !strings.Contains(out, "Test Plan") {
		t.Errorf("expected plan id and title in output, got %q", out)
	}
}

// Note: Full integration test for status would require a properly initialized
// ZQK project with objects. This is tested manually or in integration tests.
