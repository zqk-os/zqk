package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"context"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestUpdateShallowMergesMapValuedFields(t *testing.T) {
	// Not: t.Setenv(ZQK_TEST_ROOT) is process-scoped.
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	planDir := filepath.Join(processDir, "priority_plans")
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	for _, d := range []string{planDir, specsDir} {
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

	objID := "PRI-MAP-MERGE-001"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "priority_plan",
		objects.FieldKeyTitle:         "Map merge test",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyNotes: map[string]any{
			"keep_a": "alpha",
			"keep_b": "beta",
		},
	}
	if err := storage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("create: %v", err)
	}

	patch := map[string]any{"new_c": "gamma"}
	if err := storage.Update(ctx, secCtx, objID, map[string]any{objects.FieldKeyNotes: patch}); err != nil {
		t.Fatalf("update: %v", err)
	}

	out, err := storage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	notes, ok := out[objects.FieldKeyNotes].(map[string]any)
	if !ok {
		t.Fatalf("notes not map, got %T", out[objects.FieldKeyNotes])
	}
	if got, want := len(notes), 3; got != want {
		t.Fatalf("notes len: got %d want %d; %#v", got, want, notes)
	}
	if notes["keep_a"] != "alpha" || notes["keep_b"] != "beta" || notes["new_c"] != "gamma" {
		t.Fatalf("unexpected notes: %#v", notes)
	}
}

func TestApplyFieldUpdates_unsetsKey(t *testing.T) {
	existing := map[string]any{
		objects.FieldKeyBacklogItemRefs: map[string]any{},
		objects.FieldKeyTitle:           "keep",
	}
	applyFieldUpdates(existing, map[string]any{
		objects.FieldKeyBacklogItemRefs: FieldUnset,
		objects.FieldKeyTitle:           "keep",
	})
	if _, still := existing[objects.FieldKeyBacklogItemRefs]; still {
		t.Fatalf("FieldUnset must delete key, not leave {} : %#v", existing)
	}
	if existing[objects.FieldKeyTitle] != "keep" {
		t.Fatalf("title: %#v", existing[objects.FieldKeyTitle])
	}
}

func TestMergeMapPatchIntoExisting_mapAnyAny(t *testing.T) {
	existing := map[string]any{
		"meta": map[any]any{"x": 1, "y": 2},
	}
	patch := map[any]any{"y": 3, "z": 4}
	if !mergeMapPatchIntoExisting(existing, "meta", patch) {
		t.Fatal("expected merge")
	}
	meta, ok := existing["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta type %T", existing["meta"])
	}
	if meta["x"] != 1 || meta["y"] != 3 || meta["z"] != 4 {
		t.Fatalf("meta %#v", meta)
	}
}

// Regression: shallow merge keeps keys omitted from the patch. Callers that replace semantic state
// (e.g. convergence_session.after_state_snapshot) must include explicit empty slices to clear keys.
func TestMergeMapPatchIntoExisting_omittedNestedKeyPreservesStale(t *testing.T) {
	existing := map[string]any{
		objects.FieldKeyAfterStateSnapshot: map[string]any{
			objects.FieldKeyReadyForSessionCompletion: true,
			"session_completion_blocked_reasons":      []string{"no health data in window"},
		},
	}
	patch := map[string]any{
		objects.FieldKeyReadyForSessionCompletion: true,
		"lines_in_window":                         53,
	}
	if !mergeMapPatchIntoExisting(existing, objects.FieldKeyAfterStateSnapshot, patch) {
		t.Fatal("expected merge")
	}
	after := existing[objects.FieldKeyAfterStateSnapshot].(map[string]any)
	if _, still := after["session_completion_blocked_reasons"]; !still {
		t.Fatal("expected stale key to remain when patch omits it (documents merge semantics)")
	}
	patchClear := map[string]any{
		objects.FieldKeyReadyForSessionCompletion: true,
		"lines_in_window":                         53,
		"session_completion_blocked_reasons":      []string{},
	}
	if !mergeMapPatchIntoExisting(existing, objects.FieldKeyAfterStateSnapshot, patchClear) {
		t.Fatal("expected merge")
	}
	after2 := existing[objects.FieldKeyAfterStateSnapshot].(map[string]any)
	br, _ := after2["session_completion_blocked_reasons"].([]string)
	if len(br) != 0 {
		t.Fatalf("explicit empty slice should clear stale reasons: %#v", after2)
	}
}
