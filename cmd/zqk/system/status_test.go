package system

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestResolveStatusCheckBinary_PrefersProjectStable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stableDir := filepath.Join(root, paths.ProjectDataDir, "bin")
	if err := fileutil.MkdirAll(stableDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(stableDir, "zqk-stable")
	if err := fileutil.WriteFile(stable, []byte("#!/bin/sh\n"), paths.DirPerm755); err != nil {
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

func TestFormatStatusTableStatus_SubsystemHealth(t *testing.T) {
	t.Parallel()

	// 1. Healthy
	healthyData := map[string]any{
		objects.FieldKeyStatus: projectStatusInitialized,
		"system_health": map[string]any{
			objects.FieldKeyStatus: "partial_ok",
			"scheduler":            map[string]any{"running": true},
			"blocking_issues":      0,
			"warnings":             0,
		},
	}
	out := formatStatusTableStatus(healthyData)
	if !strings.Contains(out, "✅ Initialized (Healthy)") {
		t.Errorf("expected healthy initialized status, got %q", out)
	}

	// 2. Critical blocking issues - must NOT report green
	criticalData := map[string]any{
		objects.FieldKeyStatus: projectStatusInitialized,
		"system_health": map[string]any{
			objects.FieldKeyStatus: "critical",
			"scheduler":            map[string]any{"running": true},
			"blocking_issues":      3,
			"warnings":             1,
		},
	}
	outCrit := formatStatusTableStatus(criticalData)
	if strings.Contains(outCrit, "✅") {
		t.Errorf("false-green detected! Critical health must not contain ✅, got %q", outCrit)
	}
	if !strings.Contains(outCrit, "❌ Degraded / Critical (3 blocking issue(s) detected)") {
		t.Errorf("expected critical status text, got %q", outCrit)
	}

	// 3. Scheduler stopped - must report warning
	schedStoppedData := map[string]any{
		objects.FieldKeyStatus: projectStatusInitialized,
		"system_health": map[string]any{
			objects.FieldKeyStatus: "partial_ok",
			"scheduler":            map[string]any{"running": false},
			"blocking_issues":      0,
		},
	}
	outSched := formatStatusTableStatus(schedStoppedData)
	if strings.Contains(outSched, "✅") {
		t.Errorf("false-green detected! Stopped scheduler must not contain ✅, got %q", outSched)
	}
	if !strings.Contains(outSched, "⚠️  Initialized (Scheduler daemon stopped)") {
		t.Errorf("expected scheduler stopped warning, got %q", outSched)
	}
}

func TestFormatStatusTableHealth_DisplaysDetails(t *testing.T) {
	t.Parallel()

	healthData := map[string]any{
		"system_health": map[string]any{
			"scheduler":            map[string]any{"running": true},
			objects.FieldKeyStatus: "warning",
			"blocking_issues":      0,
			"warnings":             4,
		},
	}
	out := formatStatusTableHealth(healthData)
	if !strings.Contains(out, "System Health:") {
		t.Errorf("expected 'System Health:' header, got %q", out)
	}
	if !strings.Contains(out, "Scheduler: running") {
		t.Errorf("expected scheduler running, got %q", out)
	}
	if !strings.Contains(out, "Warnings: 4") {
		t.Errorf("expected warnings count, got %q", out)
	}
}

