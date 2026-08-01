package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestHashMigration_Migrate(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "hash-migration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create directory structure
	backlogDir := datacell.CellCASPrimaryDir(tempDir, "backlog")
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog dir: %v", err)
	}

	// Create test object file
	objectFile := filepath.Join(backlogDir, "ITEM-001.yaml")
	if err := os.WriteFile(objectFile, []byte("id: ITEM-001\nkind: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create object file: %v", err)
	}

	// Create old-style .hash file
	hashFile := objectFile + ".hash"
	testHash := "abc123def456"
	if err := os.WriteFile(hashFile, []byte(testHash), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create hash file: %v", err)
	}

	// Run migration (dry run)
	processRoot := datacell.ProcessPrimaryDir(tempDir)
	migration := NewHashMigration(processRoot)
	migration.SetVerbose(true)
	result, err := migration.Migrate(true)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify results
	if result.HashesMigrated != 1 {
		t.Errorf("Expected 1 hash migrated, got %d", result.HashesMigrated)
	}

	if len(result.KindsProcessed) != 1 || result.KindsProcessed[0] != "backlog_item" {
		t.Errorf("Expected backlog_item to be processed, got %v", result.KindsProcessed)
	}

	// Run actual migration
	_, err = migration.Migrate(false)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify hash index was created
	registry := NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	if err := registry.Load(); err != nil {
		t.Fatalf("Failed to load registry: %v", err)
	}

	hash := registry.GetHash("ITEM-001.yaml")
	if hash != testHash {
		t.Errorf("Expected hash %s, got %s", testHash, hash)
	}

	// Verify old hash file was removed
	if _, err := os.Stat(hashFile); !os.IsNotExist(err) {
		t.Error("Old hash file should have been removed")
	}
}

func TestHashMigration_ParseHashFile(t *testing.T) {
	root := datacell.ProcessPrimaryDir("/test/root")
	migration := NewHashMigration(root)

	tests := []struct {
		hashPath     string
		expectedKind string
		expectedFile string
	}{
		{
			hashPath:     filepath.Join(root, "backlog", "ITEM-001.yaml.hash"),
			expectedKind: "backlog_item",
			expectedFile: "ITEM-001.yaml",
		},
		{
			hashPath:     filepath.Join(root, "goals", "GOAL-001.yaml.hash"),
			expectedKind: "goal",
			expectedFile: "GOAL-001.yaml",
		},
		{
			hashPath:     filepath.Join(root, "milestones", "MIL-001.yaml.hash"),
			expectedKind: "milestone",
			expectedFile: "MIL-001.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.hashPath, func(t *testing.T) {
			kind, filename, err := migration.parseHashFile(tt.hashPath)
			if err != nil {
				t.Fatalf("parseHashFile failed: %v", err)
			}

			if kind != tt.expectedKind {
				t.Errorf("Expected kind %s, got %s", tt.expectedKind, kind)
			}

			if filename != tt.expectedFile {
				t.Errorf("Expected filename %s, got %s", tt.expectedFile, filename)
			}
		})
	}
}

func TestHashMigration_SkipExistingHashes(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "hash-migration-skip-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	backlogDir := datacell.CellCASPrimaryDir(tempDir, "backlog")
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog dir: %v", err)
	}

	// Create existing hash index with a hash
	registry := NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	registry.SetHash("ITEM-001.yaml", "existing-hash")
	if err := registry.Save(); err != nil {
		t.Fatalf("Failed to save registry: %v", err)
	}

	// Create old .hash file with same hash
	hashFile := filepath.Join(backlogDir, "ITEM-001.yaml.hash")
	if err := os.WriteFile(hashFile, []byte("existing-hash"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create hash file: %v", err)
	}

	// Run migration
	processRoot := datacell.ProcessPrimaryDir(tempDir)
	migration := NewHashMigration(processRoot)
	result, err := migration.Migrate(false)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify hash was skipped
	if result.HashesMigrated != 0 {
		t.Errorf("Expected 0 hashes migrated (should skip), got %d", result.HashesMigrated)
	}

	if result.HashesSkipped != 1 {
		t.Errorf("Expected 1 hash skipped, got %d", result.HashesSkipped)
	}
}
