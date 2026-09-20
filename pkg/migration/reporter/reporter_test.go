package reporter

import (
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReporter(t *testing.T) {
	t.Parallel()
	r := NewReporter()
	r.Start()

	// Record some activity
	r.RecordObjectScanned()
	r.RecordObjectMigrated()
	r.RecordDocumentCreated()
	r.RecordEntityCreated()
	r.RecordEdgeCreated()
	r.RecordError("parse", "BLI-001", "backlog_item", "parse error", "test.yaml")
	r.RecordWarning("test warning")
	r.RecordUnresolvedReference("MIL-999")
	r.RecordOrphanedEntity("BLI-999")

	r.Finish()

	report := r.GetReport()
	if report.ObjectsScanned != 1 {
		t.Errorf("Expected 1 object scanned, got %d", report.ObjectsScanned)
	}
	if report.ObjectsMigrated != 1 {
		t.Errorf("Expected 1 object migrated, got %d", report.ObjectsMigrated)
	}
	if len(report.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(report.Errors))
	}
	if len(report.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(report.Warnings))
	}
	if report.Duration <= 0 {
		t.Error("Expected positive duration")
	}
}

func TestGenerateJSON(t *testing.T) {
	t.Parallel()
	r := NewReporter()
	r.Start()
	r.RecordObjectMigrated()
	r.Finish()

	tmpFile := t.TempDir() + "/report.json"
	if err := r.GenerateJSON(tmpFile); err != nil {
		t.Fatalf("Failed to generate JSON report: %v", err)
	}

	// Verify file exists
	if _, err := fileutil.Stat(tmpFile); fileutil.IsNotExist(err) {
		t.Error("Report file was not created")
	}
}

func TestGenerateMarkdown(t *testing.T) {
	t.Parallel()
	r := NewReporter()
	r.Start()
	r.RecordObjectScanned()
	r.RecordObjectMigrated()
	r.RecordEntityCreated()
	r.RecordError("validation", "BLI-001", "backlog_item", "test error", "test.yaml")
	r.RecordUnresolvedReference("MIL-999")
	r.Finish()

	tmpFile := t.TempDir() + "/report.md"
	if err := r.GenerateMarkdown(tmpFile); err != nil {
		t.Fatalf("Failed to generate markdown report: %v", err)
	}

	// Verify file exists and has content
	data, err := fileutil.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read report file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Migration Report") {
		t.Error("Report should contain title")
	}
	if !strings.Contains(content, "Statistics") {
		t.Error("Report should contain statistics section")
	}
}
