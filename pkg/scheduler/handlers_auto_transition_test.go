package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCryptographicVerification(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("criteria", 2*time.Second)     //nolint:errcheck // test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("backlog_item", 2*time.Second) //nolint:errcheck // test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("test_case", 2*time.Second)    //nolint:errcheck // test cleanup
		env.Cleanup()
	}()

	provider := env.Storage.(storagepkg.ObjectStorageProvider)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "1")

	crit := map[string]any{
		objects.FieldKeyID:       "CRIT-TEST-1",
		objects.FieldKeyKind:     "criteria",
		objects.FieldKeyTitle:    "Test Criteria",
		objects.FieldKeyStatus:   "validated",
		objects.FieldKeyCategory: "functional",
	}
	storagepkg.CreateCASVisible(t, provider, ctx, secCtx, crit, "validated")

	//
	bli := map[string]any{
		objects.FieldKeyID:           "BLI-TEST-1",
		objects.FieldKeyKind:         "backlog_item",
		objects.FieldKeyTitle:        "Test BLI",
		objects.FieldKeyStatus:       "in_progress",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-TEST-1"},
	}
	storagepkg.CreateCASVisible(t, provider, ctx, secCtx, bli, objects.ObjectStatusInProgress)
	// Create test case
	tc := map[string]any{
		objects.FieldKeyID:                   "TEST-TEST-1",
		objects.FieldKeyKind:                 "test_case",
		objects.FieldKeyTitle:                "Test case",
		objects.FieldKeyStatus:               "complete",
		objects.FieldKeyBacklogItemRefs:      []any{"BLI-TEST-1"},
		objects.FieldKeyVerifiedArtifactRefs: []any{"artifact-1", "artifact-2"},
		objects.FieldKeyVerificationHash:     "dummy-hash",
	}
	storagepkg.CreateCASVisible(t, provider, ctx, secCtx, tc, objects.ObjectStatusComplete)

	// Create event data
	eventData := map[string]any{
		objects.FieldKeyKind:            "test_case",
		objects.FieldKeyStatus:          "complete",
		objects.FieldKeyBacklogItemRefs: []any{"BLI-TEST-1"},
	}

	ctxWithEvt := context.WithValue(ctx, evtDataKey{}, eventData)

	handler := NewAutoTransitionHandler(provider)
	job := &ScheduledJob{ID: "job-1"}

	if err := handler.Execute(ctxWithEvt, job); err != nil {
		t.Fatalf("handler execution failed: %v", err)
	}

	// Verify the BLI
	updatedBli, err := provider.Read(ctx, secCtx, "BLI-TEST-1")
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
