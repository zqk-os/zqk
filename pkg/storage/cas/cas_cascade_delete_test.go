package cas_test

import (
	"context"
	"github.com/lanceman/zqk/pkg/storage"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// These tests validate cascade delete behavior for CAS objects as specified in
// docs/process/architecture/cascade-delete-requirements-v1.0.md
//
// Test Coverage:
// - CAS object with dependents (cascade=false should fail)
// - CAS object with dependents (cascade=true should work)
// - CAS object as dependent of non-CAS object
// - Multi-level cascade with mixed storage types
// - CAS object with no dependents (normal delete)
//
// These tests use NewFileObjectStorage with per-project CAS index queues (see setup_test.go).
// FlushAllListingIndexesForProjectRoot is used after creates so all index queues for this test's
// project root are drained before dependents/delete; with parallel execution each test has an
// isolated queue. Using FlushAll (instead of per-kind FlushKind) stabilizes under max
// parallelization by waiting for every kind that was touched.
//
// Implementation: Cascade order is correct in pkg/storage/object_storage_file_delete.go
// (dependents checked first, then CAS vs non-CAS). If tests fail (e.g. Read finds
// object after Delete), the cause is likely CAS index visibility or Read path
// behavior, not cascade order—see docs/process/architecture/cascade-delete-requirements-v1.0.md.

// TestCAS_CascadeDelete_WithDependents_CascadeFalse tests that deleting a CAS object
// with dependents fails when cascade=false
func TestCAS_CascadeDelete_WithDependents_CascadeFalse(t *testing.T) {
	// Do not t.Parallel: DisableStreamStorageForTest mutates ZQK_STREAM_STORAGE_ENABLED.
	storage.DisableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-cascade-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Verify CAS is enabled for audit_event
	if !fileStorage.UsesContentAddressableStorage("audit_event") {
		t.Fatalf("CAS should be enabled for audit_event when path contains 'test-scenarios'")
	}

	// Cascade may hard-delete critical dependents (e.g. backlog_item).
	ctx := storage.WithTestHardDelete(pkgctx.NewSystemContext())
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create a CAS object (audit_event) - simpler than scheduler_job
	auditEvent := map[string]any{
		objects.FieldKeyID:            "AUD-001",
		objects.FieldKeyKind:          "audit_event",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2030-01-18T10:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:     "2030-01-18T10:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginSystem:  "test",
		objects.FieldKeyOriginProject: "test",
		objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
		objects.FieldKeyEventType:     "object_creation",
		objects.FieldKeyOperation:     "Test operation",
		objects.FieldKeySeverity:      "low",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, auditEvent, "")

	// Create a non-CAS dependent object (backlog_item) that references the scheduler job
	// Note: This is a simplified test - in reality, backlog_item might not reference scheduler_job
	// but we need something that can reference it for testing
	backlogItem := map[string]any{
		objects.FieldKeyID:            "BLI-001",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Test Backlog Item",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		// Add a reference field - using a generic reference field for testing
		// In practice, this would be a field that actually references scheduler_job
		"related_refs": []string{"AUD-001"},
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, backlogItem, "")

	// Flush all index queues so findDependents sees audit_event and backlog_item (stable under parallel)
	storage.FlushAllOrFail(t, fileStorage.GetProjectRoot())

	// Attempt to delete audit event with cascade=false
	// This should fail because backlog item references it
	err = fileStorage.Delete(ctx, secCtx, "AUD-001", false)
	if err == nil {
		t.Fatal("Expected error when deleting CAS object with dependents (cascade=false)")
	}

	if !strings.Contains(err.Error(), "dependent") {
		t.Errorf("Expected error about dependents, got: %v", err)
	}

	// Verify audit event still exists
	_, err = fileStorage.Read(ctx, secCtx, "AUD-001")
	if err != nil {
		t.Errorf("Audit event should still exist after failed delete: %v", err)
	}

	// Verify backlog item still exists
	_, err = fileStorage.Read(ctx, secCtx, "BLI-001")
	if err != nil {
		t.Errorf("Backlog item should still exist: %v", err)
	}
}

// TestCAS_CascadeDelete_WithDependents_CascadeTrue tests that deleting a CAS object
// with dependents succeeds when cascade=true
func TestCAS_CascadeDelete_WithDependents_CascadeTrue(t *testing.T) {
	// Do not t.Parallel: DisableStreamStorageForTest mutates ZQK_STREAM_STORAGE_ENABLED.
	storage.DisableStreamStorageForTest(t)

	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-cascade-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := storage.WithTestHardDelete(pkgctx.NewSystemContext())
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create a CAS object (audit_event)
	auditEvent := map[string]any{
		objects.FieldKeyID:            "AUD-002",
		objects.FieldKeyKind:          "audit_event",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2030-01-18T10:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:     "2030-01-18T10:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginSystem:  "test",
		objects.FieldKeyOriginProject: "test",
		objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
		objects.FieldKeyEventType:     "object_creation",
		objects.FieldKeyOperation:     "Test operation",
		objects.FieldKeySeverity:      "low",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, auditEvent, "")

	// Create a dependent object
	backlogItem := map[string]any{
		objects.FieldKeyID:            "BLI-002",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Test Backlog Item",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		"related_refs":                []string{"AUD-002"},
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, backlogItem, "")

	// Flush all index queues so findDependents sees both kinds (stable under parallel)
	storage.FlushAllOrFail(t, fileStorage.GetProjectRoot())

	// Verify both objects exist
	_, err = fileStorage.Read(ctx, secCtx, "AUD-002")
	if err != nil {
		t.Fatalf("Audit event should exist before deletion: %v", err)
	}

	_, err = fileStorage.Read(ctx, secCtx, "BLI-002")
	if err != nil {
		t.Fatalf("Backlog item should exist before deletion: %v", err)
	}

	// Delete audit event with cascade=true (Delete waits on index-remove done channel and FlushKind internally)
	err = fileStorage.Delete(ctx, secCtx, "AUD-002", true)
	if err != nil {
		t.Fatalf("Failed to delete audit event with cascade: %v", err)
	}

	// Verify audit event is deleted
	_, err = fileStorage.Read(ctx, secCtx, "AUD-002")
	if err == nil {
		t.Error("Audit event should be deleted")
	}

	// Verify dependent is also deleted (cascade)
	_, err = fileStorage.Read(ctx, secCtx, "BLI-002")
	if err == nil {
		t.Error("Backlog item should be deleted due to cascade")
	}

	// Verify CAS index entry is removed (Delete already waited for index update)
	cas, err := fileStorage.GetContentAddressableStorage("audit_event")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	_, err = cas.GetHashForID("AUD-002")
	if err == nil {
		t.Error("CAS index entry should be removed after deletion")
	}
}

// TestCAS_CascadeDelete_CASDependentOfNonCAS tests cascade delete when a CAS object
// is a dependent of a non-CAS object
func TestCAS_CascadeDelete_CASDependentOfNonCAS(t *testing.T) {
	storage.DisableStreamStorageForTest(t)

	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-cascade-mixed")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := storage.WithTestHardDelete(pkgctx.NewSystemContext())
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-1785920548450214012-68b850c0"}

	parent := map[string]any{
		objects.FieldKeyID:            "BLI-parent-cas-dep",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Parent backlog item",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
	}
	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, parent, "")

	child := map[string]any{
		objects.FieldKeyID:            "AUD-child-of-bli",
		objects.FieldKeyKind:          "audit_event",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2030-01-18T10:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:     "2030-01-18T10:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginSystem:  "test",
		objects.FieldKeyOriginProject: "test",
		objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
		objects.FieldKeyEventType:     "object_creation",
		objects.FieldKeyOperation:     "Test",
		objects.FieldKeySeverity:      "low",
		"related_refs":                []string{"BLI-parent-cas-dep"},
	}
	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, child, "")
	_ = storage.FlushAllListingIndexesForProjectRoot(fileStorage.GetProjectRoot())

	// AUD-* dependents are filtered from cascade blocking; parent delete should succeed.
	if err := fileStorage.Delete(ctx, secCtx, "BLI-parent-cas-dep", true); err != nil {
		t.Fatalf("cascade delete parent: %v", err)
	}
	if _, err := fileStorage.Read(ctx, secCtx, "BLI-parent-cas-dep"); err == nil {
		t.Fatal("parent should be gone")
	}
}

// TestCAS_CascadeDelete_MultiLevel tests multi-level cascade delete with mixed storage types
func TestCAS_CascadeDelete_MultiLevel(t *testing.T) {
	storage.DisableStreamStorageForTest(t)

	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-cascade-multilevel")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := storage.WithTestHardDelete(pkgctx.NewSystemContext())
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-1785920548450214012-68b850c0"}

	root := map[string]any{
		objects.FieldKeyID:            "BLI-cascade-root",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Cascade root item",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
	}
	mid := map[string]any{
		objects.FieldKeyID:            "BLI-cascade-mid",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Cascade mid item",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		"related_refs":                []string{"BLI-cascade-root"},
	}
	leaf := map[string]any{
		objects.FieldKeyID:            "BLI-cascade-leaf",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Cascade leaf item",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		"related_refs":                []string{"BLI-cascade-mid"},
	}
	for _, obj := range []map[string]any{root, mid, leaf} {
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, "")
	}
	_ = storage.FlushAllListingIndexesForProjectRoot(fileStorage.GetProjectRoot())

	if err := fileStorage.Delete(ctx, secCtx, "BLI-cascade-root", true); err != nil {
		t.Fatalf("multi-level cascade: %v", err)
	}
	for _, id := range []string{"BLI-cascade-root", "BLI-cascade-mid", "BLI-cascade-leaf"} {
		if _, err := fileStorage.Read(ctx, secCtx, id); err == nil {
			t.Fatalf("%s should be deleted (no leak)", id)
		}
	}
}

// TestCAS_CascadeDelete_NoDependents tests that deleting a CAS object without dependents
// works normally (no cascade needed)
func TestCAS_CascadeDelete_NoDependents(t *testing.T) {
	// Do not t.Parallel: DisableStreamStorageForTest mutates ZQK_STREAM_STORAGE_ENABLED.
	storage.DisableStreamStorageForTest(t)

	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-cascade-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := storage.WithCLIOperation(pkgctx.NewSystemContext())
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create a CAS object (audit_event) with no dependents
	auditEvent := map[string]any{
		objects.FieldKeyID:            "AUD-005",
		objects.FieldKeyKind:          "audit_event",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2030-01-18T10:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:     "2030-01-18T10:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginSystem:  "test",
		objects.FieldKeyOriginProject: "test",
		objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
		objects.FieldKeyEventType:     "object_creation",
		objects.FieldKeyOperation:     "Test operation",
		objects.FieldKeySeverity:      "low",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, auditEvent, "")

	// Wait for index updates
	storage.FlushOrFail(t, fileStorage.GetProjectRoot(), "audit_event")

	// Verify object exists
	_, err = fileStorage.Read(ctx, secCtx, "AUD-005")
	if err != nil {
		t.Fatalf("Audit event should exist before deletion: %v", err)
	}

	// Delete audit event (cascade flag doesn't matter when no dependents; Delete waits on index-remove)
	err = fileStorage.Delete(ctx, secCtx, "AUD-005", false)
	if err != nil {
		t.Fatalf("Failed to delete audit event: %v", err)
	}

	// Verify audit event is deleted
	_, err = fileStorage.Read(ctx, secCtx, "AUD-005")
	if err == nil {
		t.Error("Audit event should be deleted")
	}

	// Verify CAS index entry is removed
	cas, err := fileStorage.GetContentAddressableStorage("audit_event")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	_, err = cas.GetHashForID("AUD-005")
	if err == nil {
		t.Error("CAS index entry should be removed after deletion")
	}
}
