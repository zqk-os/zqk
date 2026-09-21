package cas_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/storage"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// These tests validate cascade delete behavior for CAS objects as specified in
// docs/architecture/cascade-delete-requirements-v1.0.md
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
// behavior, not cascade order—see docs/architecture/cascade-delete-requirements-v1.0.md.

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

	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		storage.ScrubProjectRootForTempCleanup(baseTempDir, 50, 25*time.Millisecond)
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
		"related_object_refs":         []any{"AUD-001"},
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

	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		storage.ScrubProjectRootForTempCleanup(baseTempDir, 50, 25*time.Millisecond)
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
		"related_object_refs":         []any{"AUD-002"},
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
	t.Logf("StreamStorageEnabledForKind(audit_event)=%v", storage.StreamStorageEnabledForKind("audit_event"))
	err = fileStorage.Delete(ctx, secCtx, "AUD-002", true)
	if err != nil {
		t.Fatalf("Failed to delete audit event with cascade: %v", err)
	}

	// Verify audit event is deleted
	readObj, err := fileStorage.Read(ctx, secCtx, "AUD-002")
	t.Logf("read AUD-002: obj=%v, err=%v", readObj, err)
	if err == nil {
		t.Error("Audit event should be deleted")
	}

	// Verify dependent is unlinked per VDS (association refs unlink, do not delete)
	readDep, err := fileStorage.Read(ctx, secCtx, "BLI-002")
	if err != nil {
		t.Fatalf("Backlog item should still exist after association unlink: %v", err)
	}
	refs, _ := readDep["related_object_refs"].([]any)
	if len(refs) != 0 {
		t.Errorf("Expected related_object_refs to be empty after unlink, got: %v", refs)
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

	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		storage.ScrubProjectRootForTempCleanup(baseTempDir, 50, 25*time.Millisecond)
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
		"related_object_refs":         []any{"BLI-parent-cas-dep"},
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

	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		storage.ScrubProjectRootForTempCleanup(baseTempDir, 50, 25*time.Millisecond)
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
		"related_object_refs":         []any{"BLI-cascade-root"},
	}
	leaf := map[string]any{
		objects.FieldKeyID:            "BLI-cascade-leaf",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Cascade leaf item",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		"related_object_refs":         []any{"BLI-cascade-mid"},
	}
	for _, obj := range []map[string]any{root, mid, leaf} {
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, "")
	}
	_ = storage.FlushAllListingIndexesForProjectRoot(fileStorage.GetProjectRoot())

	if err := fileStorage.Delete(ctx, secCtx, "BLI-cascade-root", true); err != nil {
		t.Fatalf("multi-level cascade: %v", err)
	}
	// Root is deleted
	if _, err := fileStorage.Read(ctx, secCtx, "BLI-cascade-root"); err == nil {
		t.Fatalf("BLI-cascade-root should be deleted")
	}
	// Per VDS, association dependents are unlinked, not deleted.
	midRead, err := fileStorage.Read(ctx, secCtx, "BLI-cascade-mid")
	if err != nil {
		t.Fatalf("BLI-cascade-mid should remain after association unlink: %v", err)
	}
	refs, _ := midRead["related_object_refs"].([]any)
	for _, ref := range refs {
		if ref == "BLI-cascade-root" {
			t.Fatalf("BLI-cascade-root should be unlinked from mid: %v", refs)
		}
	}
	if _, err := fileStorage.Read(ctx, secCtx, "BLI-cascade-leaf"); err != nil {
		t.Fatalf("BLI-cascade-leaf should remain: %v", err)
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

	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		storage.ScrubProjectRootForTempCleanup(baseTempDir, 50, 25*time.Millisecond)
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
