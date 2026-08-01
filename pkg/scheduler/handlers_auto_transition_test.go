package scheduler

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

func TestCryptographicVerification(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	provider := env.Storage.(storagepkg.ObjectStorageProvider)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create BLI
	bli := map[string]any{
		objects.FieldKeyID:     "ITEM-TEST-1",
		objects.FieldKeyKind:   "backlog_item",
		objects.FieldKeyStatus: "in_progress",
	}
	if err := provider.Create(ctx, secCtx, bli); err != nil {
		t.Fatalf("failed to create BLI: %v", err)
	}

	// Create test case
	tc := map[string]any{
		objects.FieldKeyID:                   "TEST-TEST-1",
		objects.FieldKeyKind:                 "test_case",
		objects.FieldKeyStatus:               "complete",
		objects.FieldKeyBacklogItemRefs:      []any{"ITEM-TEST-1"},
		objects.FieldKeyVerifiedArtifactRefs: []any{"artifact-1", "artifact-2"},
		objects.FieldKeyVerificationHash:     "dummy-hash",
	}
	if err := provider.Create(ctx, secCtx, tc); err != nil {
		t.Fatalf("failed to create TC: %v", err)
	}

	// Create event data
	eventData := map[string]any{
		objects.FieldKeyKind:            "test_case",
		objects.FieldKeyStatus:          "complete",
		objects.FieldKeyBacklogItemRefs: []any{"ITEM-TEST-1"},
	}

	ctxWithEvt := context.WithValue(ctx, evtDataKey{}, eventData)

	handler := NewAutoTransitionHandler(provider)
	job := &ScheduledJob{ID: "job-1"}

	if err := handler.Execute(ctxWithEvt, job); err != nil {
		t.Fatalf("handler execution failed: %v", err)
	}

	// Verify the BLI
	updatedBli, err := provider.Read(ctx, secCtx, "ITEM-TEST-1")
	if err != nil {
		t.Fatalf("failed to read updated BLI: %v", err)
	}

	if updatedBli[objects.FieldKeyStatus] != "complete" {
		t.Errorf("expected status complete, got %v", updatedBli[objects.FieldKeyStatus])
	}

	hash, ok := updatedBli[objects.FieldKeyVerificationHash].(string)
	if !ok || hash == "" {
		t.Errorf("expected non-empty verification_hash")
	}
}
