package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestRepairCASHFilenameMismatch_NoOpWhenFilenameMatchesContent(t *testing.T) {
	dir := t.TempDir()
	kindDir := datacell.CellCASPrimaryDir(dir, "backlog")
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatal(err)
	}
	content := []byte("id: X\nkind: backlog_item\n")
	h := CalculateSHA256Hash(content)
	p := filepath.Join(kindDir, h+".yaml")
	if err := fileutil.WriteSecureFile(p, content); err != nil {
		t.Fatal(err)
	}
	res, err := RepairCASHFilenameMismatch(context.Background(), dir, "backlog_item", "ITEM-test", p, &CASCorruptionRepairOptions{})
	if err != nil {
		t.Fatalf("RepairCASHFilenameMismatch: %v", err)
	}
	if res.Fixed {
		t.Fatal("expected Fixed=false when filename already matches content")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file should still exist at original path: %v", err)
	}
}

func TestBatchRepairCASMismatch(t *testing.T) {
	dir := t.TempDir()
	kindDir := datacell.CellCASPrimaryDir(dir, "backlog")
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatal(err)
	}
	content := []byte("id: X\nkind: backlog_item\n")
	h := CalculateSHA256Hash(content)
	p := filepath.Join(kindDir, h+".yaml")
	if err := fileutil.WriteSecureFile(p, content); err != nil {
		t.Fatal(err)
	}

	items := []CASCorruptionRepairItem{
		{
			Kind:            "backlog_item",
			ObjectID:        "ITEM-test",
			CorruptFilePath: p,
		},
	}

	results, err := BatchRepairCASMismatch(context.Background(), dir, items, &CASCorruptionRepairOptions{DryRun: true})
	if err != nil {
		t.Fatalf("BatchRepairCASMismatch failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0].ObjectID != "ITEM-test" {
		t.Errorf("Expected ObjectID 'ITEM-test', got '%s'", results[0].ObjectID)
	}
}
