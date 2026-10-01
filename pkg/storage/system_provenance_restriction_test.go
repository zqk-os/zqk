package storage_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupTestStorage(t *testing.T) (*storage.FileObjectStorage, string, *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-prov-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Cleanup(func() {
		_ = fos.Shutdown(context.Background())
		opts := storage.TempProjectTeardown(tmpDir, fos)
		_ = storage.RunProjectTestTeardown(opts)
		_ = fileutil.RemoveAll(tmpDir)
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	return fos, tmpDir, secCtx
}

func createTestBacklogItem(t *testing.T, fos *storage.FileObjectStorage, secCtx *pkgctx.SecurityContext, id string) map[string]any {
	t.Helper()
	ctx := context.Background()
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Initial Backlog Title",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyDescription:   "Substantive test description for CAS visibility test harness.",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyPriorityTier:  "P2",
		objects.FieldKeyPriority:      "medium",
	}
	storage.CreateCASVisible(t, fos, ctx, secCtx, obj, "planned")
	created, err := fos.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("failed to read test backlog item: %v", err)
	}
	return created
}

func TestSystemProvenanceFields_FailsClosedWithoutBreakGlass(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	ctx := context.Background()

	provenanceFields := []struct {
		field string
		value any
	}{
		{field: objects.FieldKeyCreatedAt, value: "2025-01-01T00:00:00Z"},
		{field: objects.FieldKeyCreatedBy, value: "ACC-MALICIOUS"},
		{field: objects.FieldKeyUpdatedAt, value: "2025-01-01T00:00:00Z"},
		{field: objects.FieldKeyUpdatedBy, value: "ACC-MALICIOUS"},
		{field: "cas_address", value: "cas://fake-address"},
		{field: "hash", value: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
	}

	for _, tc := range provenanceFields {
		t.Run(tc.field, func(t *testing.T) {
			objID := fmt.Sprintf("BLI-101-%s", tc.field)
			createTestBacklogItem(t, fos, secCtx, objID)

			err := fos.Update(ctx, secCtx, objID, map[string]any{
				tc.field: tc.value,
			})
			if err == nil {
				t.Fatalf("expected update of %q to fail closed without break-glass, but it succeeded", tc.field)
			}

			if !strings.Contains(err.Error(), "manual mutation of system provenance field(s)") {
				t.Errorf("error %q should mention 'manual mutation of system provenance field(s)'", err.Error())
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error %q should include violating field %q", err.Error(), tc.field)
			}
			if !strings.Contains(err.Error(), "system-computed and immutable") {
				t.Errorf("error %q should mention 'system-computed and immutable'", err.Error())
			}
		})
	}
}

func TestSystemProvenanceFields_MultipleViolatingFields(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	ctx := context.Background()

	objID := "BLI-102"
	createTestBacklogItem(t, fos, secCtx, objID)

	err := fos.Update(ctx, secCtx, objID, map[string]any{
		objects.FieldKeyCreatedAt: "2025-01-01T00:00:00Z",
		"cas_address":             "cas://spoofed-addr",
		"hash":                    "spoofed-hash",
	})
	if err == nil {
		t.Fatal("expected update with multiple provenance fields to fail closed, but it succeeded")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "manual mutation of system provenance field(s)") {
		t.Errorf("error %q should mention 'manual mutation of system provenance field(s)'", errMsg)
	}
	if !strings.Contains(errMsg, "created_at") || !strings.Contains(errMsg, "cas_address") || !strings.Contains(errMsg, "hash") {
		t.Errorf("error %q should mention all violating fields (created_at, cas_address, hash)", errMsg)
	}
}

func TestSystemProvenanceFields_ExpectedUpdatedAtPermitted(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	ctx := context.Background()

	objID := "BLI-103"
	created := createTestBacklogItem(t, fos, secCtx, objID)
	curUpdatedAt, ok := created[objects.FieldKeyUpdatedAt].(string)
	if !ok || curUpdatedAt == "" {
		t.Fatalf("expected created object to have updated_at, got %v", created[objects.FieldKeyUpdatedAt])
	}

	// Update with expected_updated_at should succeed as it is optimistic locking, not a provenance override
	err := fos.Update(ctx, secCtx, objID, map[string]any{
		storage.FieldKeyExpectedUpdatedAt: curUpdatedAt,
		objects.FieldKeyTitle:             "Updated Title with Expected Timestamp",
	})
	if err != nil {
		t.Fatalf("expected update with expected_updated_at to succeed, got: %v", err)
	}

	updated, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}
	if updated[objects.FieldKeyTitle] != "Updated Title with Expected Timestamp" {
		t.Errorf("expected title to be updated, got %v", updated[objects.FieldKeyTitle])
	}
}

func TestSystemProvenanceFields_BreakGlassPermitted(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(context.Background()), "authorized test emergency provenance fix")

	provenanceFields := []struct {
		name  string
		field string
		value any
	}{
		{name: "created_at", field: objects.FieldKeyCreatedAt, value: "2024-05-15T12:00:00Z"},
		{name: "created_by", field: objects.FieldKeyCreatedBy, value: "ACC-MIGRATED-ADMIN"},
		{name: "updated_at", field: objects.FieldKeyUpdatedAt, value: "2024-06-01T15:30:00Z"},
		{name: "updated_by", field: objects.FieldKeyUpdatedBy, value: "ACC-BREAK-GLASS-OPERATOR"},
		{name: "cas_address", field: "cas_address", value: "cas://verified-migration-address"},
		{name: "hash", field: "hash", value: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
	}

	for _, tc := range provenanceFields {
		t.Run(tc.name, func(t *testing.T) {
			objID := "BLI-201-" + tc.name
			createTestBacklogItem(t, fos, secCtx, objID)

			err := fos.Update(bgCtx, secCtx, objID, map[string]any{
				tc.field: tc.value,
			})
			if err != nil {
				t.Fatalf("expected update of %q with break-glass to succeed, got: %v", tc.field, err)
			}

			readBack, err := fos.Read(bgCtx, secCtx, objID)
			if err != nil {
				t.Fatalf("failed to read back object: %v", err)
			}

			if readBack[tc.field] != tc.value {
				t.Errorf("field %q: expected value %v, got %v", tc.field, tc.value, readBack[tc.field])
			}
		})
	}
}

func TestSystemProvenanceFields_UnchangedProvenanceFieldsPermitted(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	ctx := context.Background()

	objID := "BLI-301"
	created := createTestBacklogItem(t, fos, secCtx, objID)

	// Simulate full object read-modify-write where unchanged provenance fields are present
	created[objects.FieldKeyTitle] = "Updated Backlog Item Title"
	err := fos.Update(ctx, secCtx, objID, created)
	if err != nil {
		t.Fatalf("expected read-modify-write update with unchanged provenance to succeed, got: %v", err)
	}

	readBack, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read back object: %v", err)
	}
	if readBack[objects.FieldKeyTitle] != "Updated Backlog Item Title" {
		t.Errorf("expected updated title, got %v", readBack[objects.FieldKeyTitle])
	}
}
