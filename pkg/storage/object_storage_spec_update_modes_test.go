package storage

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestSpecDrivenUpdateModes(t *testing.T) {
	tmpDir := t.TempDir()

	MustEnsureProcessSpecsLayoutForTest(t, tmpDir)

	f, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	t.Cleanup(func() { _ = RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)) })

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Goal has an immutable field (origin_project) and an append-only field (status_history if it were there, but let's test a known field or just mock it)
	// We'll use 'milestone' which has an immutable field 'origin_system' and 'kind'

	id := "MIL-1"
	kind := "milestone"

	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          kind,
		objects.FieldKeyTitle:         "Test Milestone",
		objects.FieldKeyOriginSystem:  "system-1",
		objects.FieldKeyStatusHistory: []any{"draft"},
		objects.FieldKeyArtifacts:     []any{"a1"},
	}

	if err := f.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Try to update immutable field
	updates := map[string]any{
		objects.FieldKeyOriginSystem:  "system-2",          // immutable
		objects.FieldKeyTitle:         "Updated Milestone", // mutable
		objects.FieldKeyArtifacts:     []any{"a2"},         // mutable list
		objects.FieldKeyStatusHistory: []any{"active"},     // append-only list
	}

	if err := f.Update(ctx, secCtx, id, updates); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Read and verify
	updated, err := f.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if updated[objects.FieldKeyOriginSystem] != "system-1" {
		t.Errorf("Expected origin_system to remain 'system-1' (immutable), got %v", updated[objects.FieldKeyOriginSystem])
	}

	if updated[objects.FieldKeyTitle] != "Updated Milestone" {
		t.Errorf("Expected title to be updated, got %v", updated[objects.FieldKeyTitle])
	}

	artifacts := updated[objects.FieldKeyArtifacts].([]any)
	if len(artifacts) != 1 || artifacts[0] != "a2" {
		t.Errorf("Expected artifacts to be replaced (mutable), got %v", updated[objects.FieldKeyArtifacts])
	}

	statusHistory := updated[objects.FieldKeyStatusHistory].([]any)
	if len(statusHistory) != 2 || statusHistory[0] != "draft" || statusHistory[1] != "active" {
		t.Errorf("Expected status_history to be appended to (append-only), got %v", updated[objects.FieldKeyStatusHistory])
	}
}
