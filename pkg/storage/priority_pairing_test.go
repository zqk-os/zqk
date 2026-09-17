package storage

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestBacklogItemPriorityPairing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	bliDir := filepath.Join(processDir, "backlog")
	planDir := filepath.Join(processDir, "priority_plans")
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	for _, d := range []string{bliDir, planDir, specsDir} {
		if err := fileutil.EnsureDir(d); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		_ = RunProjectTestTeardown(opts)
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// 1. Create BLI with only priority="high" -> should auto-populate priority_tier="P1"
	bliID := "BLI-1785008248438506000-11111111"
	bli := map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Priority pairing test item",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyPriority:      "high",
		objects.FieldKeyDescription:   "Test description for priority pairing",
	}

	if err := storage.Create(ctx, secCtx, bli); err != nil {
		t.Fatalf("create: %v", err)
	}

	readObj, err := storage.Read(ctx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if readObj[objects.FieldKeyPriority] != "high" {
		t.Errorf("got priority=%v, want 'high'", readObj[objects.FieldKeyPriority])
	}
	if readObj[objects.FieldKeyPriorityTier] != "P1" {
		t.Errorf("got priority_tier=%v, want 'P1'", readObj[objects.FieldKeyPriorityTier])
	}

	// 2. Update only priority="critical" -> should auto-update priority_tier="P0"
	if err := storage.Update(ctx, secCtx, bliID, map[string]any{
		objects.FieldKeyPriority: "critical",
	}); err != nil {
		t.Fatalf("update priority: %v", err)
	}

	readObj, err = storage.Read(ctx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read after update 1: %v", err)
	}
	if readObj[objects.FieldKeyPriority] != "critical" {
		t.Errorf("got priority=%v, want 'critical'", readObj[objects.FieldKeyPriority])
	}
	if readObj[objects.FieldKeyPriorityTier] != "P0" {
		t.Errorf("got priority_tier=%v, want 'P0'", readObj[objects.FieldKeyPriorityTier])
	}

	// 3. Update only priority_tier="P2" -> should auto-update priority="medium"
	if err := storage.Update(ctx, secCtx, bliID, map[string]any{
		objects.FieldKeyPriorityTier: "P2",
	}); err != nil {
		t.Fatalf("update priority_tier: %v", err)
	}

	readObj, err = storage.Read(ctx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read after update 2: %v", err)
	}
	if readObj[objects.FieldKeyPriority] != "medium" {
		t.Errorf("got priority=%v, want 'medium'", readObj[objects.FieldKeyPriority])
	}
	if readObj[objects.FieldKeyPriorityTier] != "P2" {
		t.Errorf("got priority_tier=%v, want 'P2'", readObj[objects.FieldKeyPriorityTier])
	}

	// 4. Update both with legitimate values -> should preserve both
	if err := storage.Update(ctx, secCtx, bliID, map[string]any{
		objects.FieldKeyPriority:     "low",
		objects.FieldKeyPriorityTier: "P3",
	}); err != nil {
		t.Fatalf("update both: %v", err)
	}

	readObj, err = storage.Read(ctx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read after update 3: %v", err)
	}
	if readObj[objects.FieldKeyPriority] != "low" {
		t.Errorf("got priority=%v, want 'low'", readObj[objects.FieldKeyPriority])
	}
	if readObj[objects.FieldKeyPriorityTier] != "P3" {
		t.Errorf("got priority_tier=%v, want 'P3'", readObj[objects.FieldKeyPriorityTier])
	}
}
