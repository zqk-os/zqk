package storage_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
)

// CRIT-1789663108482255000-40a4b771: Functional Acceptance
// Verifies that counting objects with default namespace isolation uses the fast CAS index path
// and matches unfiltered counts without triggering the O(N) worker YAML unmarshal parse loop.
func TestAcceleratedObjectCount_FunctionalAcceptance(t *testing.T) {
	testRoot := filepath.Join(t.TempDir(), "test-scenarios", "count-fastpath-acceptance")
	storage.MustEnsureProcessSpecsLayout(t, testRoot)

	store, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	defer func() { _ = store.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, store)); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-1785920548450214012-68b850c0"}
	ns := validation.DefaultNamespaceKernel

	const objectCount = 15
	for i := 0; i < objectCount; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("BLI-accel-%04d", i),
			objects.FieldKeyKind:          objects.KindBacklogItem,
			objects.FieldKeyTitle:         fmt.Sprintf("Accelerated count item %d", i),
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyNamespaceID:   ns,
		}
		storage.CreateCASVisible(t, store, ctx, secCtx, obj, objects.ObjectStatusValidated)
	}

	// 1. Unfiltered count
	unfilteredCount, err := store.Count(ctx, secCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
	})
	if err != nil {
		t.Fatalf("Unfiltered count failed: %v", err)
	}
	if unfilteredCount != objectCount {
		t.Fatalf("Unfiltered count: got %d, want %d", unfilteredCount, objectCount)
	}

	// 2. Default namespace filter (injected by bare 'zqk object count')
	scopedCount, err := store.Count(ctx, secCtx, storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyNamespaceID: ns},
	})
	if err != nil {
		t.Fatalf("Scoped count failed: %v", err)
	}
	if scopedCount != unfilteredCount {
		t.Fatalf("Scoped count mismatch: got %d, want %d", scopedCount, unfilteredCount)
	}
}

// CRIT-1789663108482256000-4be6433a: Boundary & Error Handling
// Verifies namespace boundaries across organizational kinds, non-existent namespaces,
// and context cancellations.
func TestAcceleratedObjectCount_BoundaryAndErrorHandling(t *testing.T) {
	testRoot := filepath.Join(t.TempDir(), "test-scenarios", "count-fastpath-boundary")
	storage.MustEnsureProcessSpecsLayout(t, testRoot)

	store, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	defer func() { _ = store.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, store)); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-1785920548450214012-68b850c0"}

	// 1. Organizational kinds with zqk:kernel namespace filter must immediately return 0 without error
	orgCount, err := store.Count(ctx, secCtx, storage.ListFilter{
		Kind:    objects.KindOrganization,
		Filters: map[string]any{objects.FieldKeyNamespaceID: "zqk:kernel"},
	})
	if err != nil {
		t.Fatalf("Organizational count with kernel namespace failed: %v", err)
	}
	if orgCount != 0 {
		t.Fatalf("Expected orgCount=0 for zqk:kernel, got %d", orgCount)
	}

	// 2. Organizational kinds with domain:organizational namespace filter on empty kind returns 0
	orgDomainCount, err := store.Count(ctx, secCtx, storage.ListFilter{
		Kind:    objects.KindOrganization,
		Filters: map[string]any{objects.FieldKeyNamespaceID: "domain:organizational"},
	})
	if err != nil {
		t.Fatalf("Organizational count with domain namespace failed: %v", err)
	}
	if orgDomainCount != 0 {
		t.Fatalf("Expected orgDomainCount=0 for empty kind, got %d", orgDomainCount)
	}

	// 3. Cancelled context must error immediately
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.Count(cancelledCtx, secCtx, storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyNamespaceID: "zqk:kernel"},
	})
	if err == nil {
		t.Fatal("Expected error for cancelled context, got nil")
	}
}

// CRIT-1789663108482257000-f20e2c1a: Integration & Conformance
// Verifies that count operations with non-fastpath filters emit progress callbacks and touch activity.
func TestAcceleratedObjectCount_IntegrationAndConformance(t *testing.T) {
	testRoot := filepath.Join(t.TempDir(), "test-scenarios", "count-fastpath-integration")
	storage.MustEnsureProcessSpecsLayout(t, testRoot)

	store, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	defer func() { _ = store.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, store)); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-1785920548450214012-68b850c0"}
	ctx := pkgctx.NewSystemContext()

	// Seed objects with mixed statuses
	for i := 0; i < 5; i++ {
		status := objects.ObjectStatusExploring
		if i%2 == 0 {
			status = objects.ObjectStatusValidated
		}
		obj := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("BLI-prog-%04d", i),
			objects.FieldKeyKind:          objects.KindBacklogItem,
			objects.FieldKeyTitle:         fmt.Sprintf("Progress item %d", i),
			objects.FieldKeyStatus:        status,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyNamespaceID:   "zqk:kernel",
		}
		storage.CreateCASVisible(t, store, ctx, secCtx, obj, status)
	}

	var progressCallCount int64
	progressFn := func(phase, message string) {
		atomic.AddInt64(&progressCallCount, 1)
	}

	progressCtx := pkgctx.WithValidationProgress(ctx, progressFn)

	// Filter on status (requires countWithFilters path)
	filterCtx, cancel := context.WithTimeout(progressCtx, 10*time.Second)
	defer cancel()

	matchedCount, err := store.Count(filterCtx, secCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusValidated,
		},
	})
	if err != nil {
		t.Fatalf("Filtered count failed: %v", err)
	}
	if matchedCount != 3 {
		t.Fatalf("Expected matchedCount=3, got %d", matchedCount)
	}

	calls := atomic.LoadInt64(&progressCallCount)
	if calls < 1 {
		t.Fatalf("Expected at least 1 progress callback during filtered count, got %d", calls)
	}
}
