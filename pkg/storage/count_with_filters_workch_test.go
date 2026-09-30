package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/validation"
)

// TestCountWithFilters_NoDeadlockWhenFilesExceedWorkerBuffer reproduces the
// enqueue-before-workers deadlock: workCh buffered to maxWorkers*2 blocked when
// file count exceeded the buffer (default namespace-scoped object count).
func TestCountWithFilters_NoDeadlockWhenFilesExceedWorkerBuffer(t *testing.T) {
	t.Setenv(zqkenv.ListReadWorkers().Name(), "4") // buffer would be 8 if sized to maxWorkers*2

	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "count-filter-workch")
	mustEnsureProcessSpecsLayout(t, testRoot)

	store, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	defer func() { _ = store.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(testRoot, store)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-SYSTEM"}
	ns := validation.DefaultNamespaceKernel

	const n = 20 // > 4*2
	for i := 0; i < n; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("BLI-count-workch-%04d", i),
			objects.FieldKeyKind:          objects.KindBacklogItem,
			objects.FieldKeyTitle:         fmt.Sprintf("Count workch %d", i),
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyNamespaceID:   ns,
		}
		CreateCASVisible(t, store, ctx, secCtx, obj, objects.ObjectStatusValidated)
	}

	countCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	count, err := store.Count(countCtx, secCtx, ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyNamespaceID: ns},
	})
	if err != nil {
		t.Fatalf("Count with namespace filter: %v (likely workCh deadlock)", err)
	}
	if count < n {
		t.Fatalf("Count = %d, want >= %d", count, n)
	}
}
