package hostservice

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRuntimeWranglerStatus(t *testing.T) {
	tempDir := t.TempDir()
	wrangler := NewRuntimeWrangler(tempDir)

	report, err := wrangler.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.ProjectRoot != tempDir {
		t.Errorf("ProjectRoot = %q, want %q", report.ProjectRoot, tempDir)
	}

	if len(report.Units) < 5 {
		t.Errorf("expected at least 5 units in status report, got %d", len(report.Units))
	}

	// Verify expected unit names are present
	unitNames := make(map[string]bool)
	for _, u := range report.Units {
		unitNames[u.Name] = true
	}

	expected := []string{"scheduler", "privileged-writer", "mcp-daemon", "mesh-night-duty", "mesh-tpm-agy-watchdog"}
	for _, exp := range expected {
		if !unitNames[exp] {
			t.Errorf("expected unit %q in report, not found", exp)
		}
	}

	// Verify JSON format
	b, err := FormatReportJSON(report)
	if err != nil {
		t.Fatalf("FormatReportJSON error = %v", err)
	}
	if len(b) == 0 {
		t.Errorf("expected non-empty JSON report")
	}
}

func TestRuntimeWranglerStopSticky(t *testing.T) {
	tempDir := t.TempDir()
	wrangler := NewRuntimeWrangler(tempDir)

	// Create fake scheduler pid
	pidDir := filepath.Join(tempDir, ".zqk", "state")
	_ = fileutil.MkdirAll(pidDir, 0755)
	_ = fileutil.WriteFile(filepath.Join(pidDir, "scheduler.pid"), []byte("9999999"), 0644)

	if err := wrangler.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Verify pid file removed
	if _, err := fileutil.Stat(filepath.Join(pidDir, "scheduler.pid")); !fileutil.IsNotExist(err) {
		t.Errorf("expected scheduler.pid to be removed after stop")
	}
}
