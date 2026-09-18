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

func TestPassiveTestSweeper(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("criteria", 2*time.Second)
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("backlog_item", 2*time.Second)
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("test_case", 2*time.Second)
		env.Cleanup()
	}()

	provider := env.Storage.(storagepkg.ObjectStorageProvider)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "1")

	critID := "CRIT-SWEEP-001"
	crit := map[string]any{
		objects.FieldKeyID:       critID,
		objects.FieldKeyKind:     "criteria",
		objects.FieldKeyTitle:    "Passive Sweeper Test Criteria",
		objects.FieldKeyStatus:   objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyCategory: "functional",
	}
	storagepkg.CreateCASVisible(t, provider, ctx, secCtx, crit, objects.ObjectStatusAwaitingVerification)

	bliID := "BLI-SWEEP-001"
	bli := map[string]any{
		objects.FieldKeyID:           bliID,
		objects.FieldKeyKind:         "backlog_item",
		objects.FieldKeyTitle:        "Passive Sweeper Test BLI",
		objects.FieldKeyStatus:       objects.ObjectStatusInProgress,
		objects.FieldKeyCriteriaRefs: []any{critID},
	}
	storagepkg.CreateCASVisible(t, provider, ctx, secCtx, bli, objects.ObjectStatusInProgress)

	tcID := "TST-SWEEP-001"
	tc := map[string]any{
		objects.FieldKeyID:              tcID,
		objects.FieldKeyKind:            "test_case",
		objects.FieldKeyTitle:           "Passive Sweeper Test Case",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyBacklogItemRefs: []any{bliID},
		objects.FieldKeyCriteriaRefs:    []any{critID},
		"verification_suites": []any{
			critID + ": true",
		},
	}
	storagepkg.CreateCASVisible(t, provider, ctx, secCtx, tc, objects.ObjectStatusActive)

	handler := NewPassiveTestSweeperHandler(provider, env.TestRoot, nil)
	job := &ScheduledJob{
		ID:      "SCH-passive-test-sweeper",
		JobType: JobTypePassiveTestSweeper,
	}

	if err := handler.Execute(ctx, job); err != nil {
		t.Fatalf("PassiveTestSweeperHandler.Execute failed: %v", err)
	}

	// Verify that the criterion transitioned to complete via Shockwave catalyst
	updatedCrit, err := provider.Read(ctx, secCtx, critID)
	if err != nil {
		t.Fatalf("failed to read updated criteria: %v", err)
	}

	status, _ := updatedCrit[objects.FieldKeyStatus].(string)
	if status != objects.ObjectStatusComplete {
		t.Errorf("expected criteria status %q, got %q", objects.ObjectStatusComplete, status)
	}
}
