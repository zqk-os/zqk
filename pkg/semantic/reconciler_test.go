package semantic

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func TestSemanticReconciler_CheckDrift_MTime(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "reconciler-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceFile := filepath.Join(tmpDir, "source.json")
	if err := fileutil.WriteSecureFile(sourceFile, []byte(`{"a": 1}`)); err != nil {
		t.Fatal(err)
	}

	info, _ := os.Stat(sourceFile)
	initialMTime := zqktime.FormatRFC3339UTC(info.ModTime())

	store := &mockStorage{}
	reconciler := NewSemanticReconciler(store)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Initial Import (no hash/mtime in store)
	obj := map[string]any{
		objects.FieldKeyID:         "IMPTRK-001",
		objects.FieldKeyKind:       objects.KindImportTracking,
		objects.FieldKeySourceFile: sourceFile,
	}
	_ = store.Create(ctx, secCtx, obj)

	// 2. First Drift Check - should calculate hash and mtime
	// Drift is expected because storedHash is empty
	results, err := reconciler.CheckDrift(ctx, secCtx)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}
	if len(results) != 1 || !results[0].Drifted {
		t.Errorf("expected drift (first discovery), got %+v", results)
	}

	// Verify store was updated
	updatedObj := store.objects[0]
	storedMTime := updatedObj[objects.FieldKeySourceMtime].(string)
	if storedMTime != initialMTime {
		t.Errorf("expected stored mtime %s, got %s", initialMTime, storedMTime)
	}

	// 3. Second Drift Check (no change) - should skip hashing
	// We'll modify the mock store to "count" hash calls if we really wanted to be surgical,
	// but for now we verify it still reports no drift.
	results, err = reconciler.CheckDrift(ctx, secCtx)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}
	if results[0].Drifted {
		t.Errorf("expected no drift when file is unchanged")
	}

	// 4. Change File and verify drift
	time.Sleep(1 * time.Second) // Ensure MTime changes
	if err := fileutil.WriteSecureFile(sourceFile, []byte(`{"a": 2}`)); err != nil {
		t.Fatal(err)
	}

	results, err = reconciler.CheckDrift(ctx, secCtx)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}
	if !results[0].Drifted {
		t.Errorf("expected drift detected after file change")
	}

	// Verify MTime was updated in store
	info2, _ := os.Stat(sourceFile)
	newMTime := zqktime.FormatRFC3339UTC(info2.ModTime())
	if store.objects[0][objects.FieldKeySourceMtime].(string) != newMTime {
		t.Errorf("expected updated mtime %s", newMTime)
	}
}
