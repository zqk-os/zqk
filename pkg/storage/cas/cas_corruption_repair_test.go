package cas_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/storage"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestRepairCASHFilenameMismatch_NoOpWhenFilenameMatchesContent(t *testing.T) {
	dir := t.TempDir()
	kindDir := datacell.CellCASPrimaryDir(dir, "backlog")
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatal(err)
	}
	content := []byte("id: X\nkind: backlog_item\n")
	h := storage.CalculateSHA256Hash(content)
	p := filepath.Join(kindDir, h+".yaml")
	if err := fileutil.WriteSecureFile(p, content); err != nil {
		t.Fatal(err)
	}
	res, err := caspkg.RepairCASHFilenameMismatch(context.Background(), dir, "backlog_item", "BLI-test", p, &caspkg.CASCorruptionRepairOptions{})
	if err != nil {
		t.Fatalf("caspkg.RepairCASHFilenameMismatch: %v", err)
	}
	if res.Fixed {
		t.Fatal("expected Fixed=false when filename already matches content")
	}
	if _, err := fileutil.Stat(p); err != nil {
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
	h := storage.CalculateSHA256Hash(content)
	p := filepath.Join(kindDir, h+".yaml")
	if err := fileutil.WriteSecureFile(p, content); err != nil {
		t.Fatal(err)
	}

	items := []caspkg.CASCorruptionRepairItem{
		{
			Kind:            "backlog_item",
			ObjectID:        "BLI-test",
			CorruptFilePath: p,
		},
	}

	results, err := caspkg.BatchRepairCASMismatch(context.Background(), dir, items, &caspkg.CASCorruptionRepairOptions{DryRun: true})
	if err != nil {
		t.Fatalf("caspkg.BatchRepairCASMismatch failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0].ObjectID != "BLI-test" {
		t.Errorf("Expected ObjectID 'BLI-test', got '%s'", results[0].ObjectID)
	}
}

// TRACK: BLI-CEF-R14-RCV-CAS-REPAIR-001 / CRIT-CEF-R14-RCV-CAS-REPAIR-001 / REQ-CEF-R14-RCV-SEC-001
func TestRepairCASHFilenameMismatch_RecoversCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	kindDir := datacell.CellCASPrimaryDir(dir, "backlog")
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatal(err)
	}

	// 1. Create a corrupted CAS file: filename is fakeHash, but content corresponds to correctHash
	fakeHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	content := []byte("id: BLI-TEST-REPAIR\nkind: backlog_item\ntitle: Repaired Object\nstatus: planned\n")
	correctHash := storage.CalculateSHA256Hash(content)
	corruptPath := filepath.Join(kindDir, fakeHash+".yaml")

	if err := fileutil.WriteSecureFile(corruptPath, content); err != nil {
		t.Fatal(err)
	}

	quarantineDir := filepath.Join(dir, ".zqk", "quarantine")

	// 2. Run caspkg.RepairCASHFilenameMismatch to repair the corruption
	opts := &caspkg.CASCorruptionRepairOptions{
		QuarantineDir: quarantineDir,
		DryRun:        false,
	}
	res, err := caspkg.RepairCASHFilenameMismatch(context.Background(), dir, "backlog_item", "BLI-TEST-REPAIR", corruptPath, opts)
	if err != nil {
		t.Fatalf("caspkg.RepairCASHFilenameMismatch: %v", err)
	}

	if !res.Fixed {
		t.Fatalf("expected Fixed=true, got false")
	}
	expectedNewPath := filepath.Join(kindDir, correctHash+".yaml")
	if res.NewFilePath != expectedNewPath {
		t.Errorf("expected NewFilePath %s, got %s", expectedNewPath, res.NewFilePath)
	}

	// 3. Assert old corrupt file is gone and correctly hashed file is present with identical content
	if _, err := fileutil.Stat(corruptPath); !fileutil.IsNotExist(err) {
		t.Fatalf("corrupt file should have been removed/quarantined from %s", corruptPath)
	}
	repairedData, err := fileutil.ReadFile(res.NewFilePath)
	if err != nil {
		t.Fatalf("failed to read repaired file at %s: %v", res.NewFilePath, err)
	}
	if string(repairedData) != string(content) {
		t.Errorf("content corrupted during repair: want %s, got %s", string(content), string(repairedData))
	}

	// 4. Assert second repair run is a no-op (idempotency)
	res2, err := caspkg.RepairCASHFilenameMismatch(context.Background(), dir, "backlog_item", "BLI-TEST-REPAIR", res.NewFilePath, opts)
	if err != nil {
		t.Fatalf("second repair run failed: %v", err)
	}
	if res2.Fixed {
		t.Fatalf("expected Fixed=false on already repaired file, got true")
	}
}
