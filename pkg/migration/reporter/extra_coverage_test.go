// BLI-STARTER-COMMUNITY-044 / PRI-STARTER-COMMUNITY-044 coverage elevation
package reporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExtraReporterRecordsAndFormats(t *testing.T) {
	r := NewReporter()
	r.Start()
	r.RecordObjectScanned()
	r.RecordObjectMigrated()
	r.RecordDocumentCreated()
	r.RecordEntityCreated()
	r.RecordEdgeCreated()
	r.RecordError("parse", "BLI-1", "backlog_item", "boom", "a.yaml")
	r.RecordWarning("watch this")
	r.RecordUnresolvedReference("MIL-9")
	r.RecordOrphanedEntity("ENT-9")
	r.RecordDuplicateID("BLI-1")
	r.RecordInvalidEdgeType("weird")
	r.RecordMissingRequiredField("BLI-2", "backlog_item", "title", "b.yaml")
	r.Finish()
	rep := r.GetReport()
	if rep.ObjectsScanned != 1 || len(rep.Errors) != 1 || len(rep.Warnings) != 1 {
		t.Fatalf("report = %#v", rep)
	}

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "r.json")
	if err := r.GenerateJSON(jsonPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded MigrationReport
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	mdPath := filepath.Join(dir, "r.md")
	if err := r.GenerateMarkdown(mdPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Fatal(err)
	}
	if err := r.GenerateJSON(filepath.Join(dir, "missing", "no.json")); err == nil {
		t.Fatal("expected json write err")
	}
	if err := r.GenerateMarkdown(filepath.Join(dir, "missing", "no.md")); err == nil {
		t.Fatal("expected md write err")
	}
}
