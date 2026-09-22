// BLI-STARTER-COMMUNITY-059 / PRI-STARTER-COMMUNITY-059 coverage elevation
package reporter

import (
	"path/filepath"
	"testing"
)

func TestExtraReporterValidationMarkdownAndWriteErrors(t *testing.T) {
	r := NewReporter()
	r.Start()
	r.RecordObjectScanned()
	r.RecordObjectMigrated()
	r.RecordDocumentCreated()
	r.RecordEntityCreated()
	r.RecordEdgeCreated()
	r.RecordError("write", "ID-1", "kind", "boom", "a.yaml")
	r.RecordWarning("slow")
	r.RecordUnresolvedReference("REF-1")
	r.RecordOrphanedEntity("ENT-1")
	r.RecordDuplicateID("DUP-1")
	r.RecordInvalidEdgeType("weird")
	r.RecordMissingRequiredField("ID-2", "kind", "title", "b.yaml")
	r.Finish()
	_ = r.GetReport()

	dir := t.TempDir()
	if err := r.GenerateJSON(filepath.Join(dir, "report.json")); err != nil {
		t.Fatal(err)
	}
	if err := r.GenerateMarkdown(filepath.Join(dir, "report.md")); err != nil {
		t.Fatal(err)
	}
	if err := r.GenerateJSON("/no/such/dir/report.json"); err == nil {
		t.Fatal("expected json write error")
	}
	if err := r.GenerateMarkdown("/no/such/dir/report.md"); err == nil {
		t.Fatal("expected markdown write error")
	}
}
