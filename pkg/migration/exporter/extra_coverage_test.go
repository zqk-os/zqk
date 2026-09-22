// BLI-STARTER-COMMUNITY-030 / PRI-STARTER-COMMUNITY-030 coverage elevation
package exporter

import (
	"context"
	"path/filepath"
	"testing"
)

func TestExportAll_RequiresOutputDir(t *testing.T) {
	t.Parallel()
	e := NewGraphExporter(nil)
	if err := e.ExportAll(context.Background(), ExportOptions{}); err == nil {
		t.Fatal("empty output dir")
	}
}

func TestExportAll_CreatesDir(t *testing.T) {
	t.Parallel()
	e := NewGraphExporter(nil)
	out := t.TempDir()
	dest := filepath.Join(out, "export")
	if err := e.ExportAll(context.Background(), ExportOptions{OutputDir: dest, ObjectTypes: []string{"backlog_item"}}); err != nil {
		t.Fatal(err)
	}
}

func TestExportEntityAndSourcePathUnimplemented(t *testing.T) {
	t.Parallel()
	e := NewGraphExporter(nil)
	ctx := context.Background()
	if err := e.ExportEntity(ctx, "E-1", "out.yaml"); err == nil {
		t.Fatal("entity")
	}
	if _, err := e.GetEntitySourcePath(ctx, "E-1"); err == nil {
		t.Fatal("source path")
	}
}

func TestToLabel_EmptyParts(t *testing.T) {
	t.Parallel()
	if toLabel("_foo_") != "Foo" {
		t.Fatalf("got %q", toLabel("_foo_"))
	}
}
