package system

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TRACK: BLI-SYSTEM-STATUS-CHECK-PATH-HONEST-001
func TestResolveStatusCheckBinary_PrefersProjectStable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stableDir := filepath.Join(root, paths.ProjectDataDir, "bin")
	if err := fileutil.MkdirAll(stableDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(stableDir, "zqk-stable")
	if err := fileutil.WriteFile(stable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveStatusCheckBinary(root)
	if got != stable {
		t.Fatalf("expected project stable %q, got %q", stable, got)
	}
}

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
	// Plan with backlog_items (BLI-721 enhancement)
	statusData := map[string]any{
		"current_priority_plan": map[string]any{
			objects.FieldKeyID:    "PRI-219",
			objects.FieldKeyTitle: "Process & System",
			"backlog_items":       29,
		},
	}
	out := formatStatusTablePlan(statusData)
	if out == emptyValue {
		t.Fatal("formatStatusTablePlan returned empty")
	}
	if !strings.Contains(out, "PRI-219") || !strings.Contains(out, "Process & System") {
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
			objects.FieldKeyID:    "PRI-001",
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
	if !strings.Contains(out, "PRI-001") || !strings.Contains(out, "Test Plan") {
		t.Errorf("expected plan id and title in output, got %q", out)
	}
}

// Note: Full integration test for status would require a properly initialized
// ZQK project with objects. This is tested manually or in integration tests.
