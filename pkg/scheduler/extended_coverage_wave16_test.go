package scheduler

import (
	"context"
	"os"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_TriggerQueue_PeekAndStructs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-tq-peek-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	q := NewJobTriggerQueue(tmpDir)

	// 1. PeekTriggerRequests when queue does not exist
	peekEmpty, err := q.PeekTriggerRequests()
	if err != nil || len(peekEmpty) != 0 {
		t.Errorf("expected empty peek on nonexistent queue, got len %d, err %v", len(peekEmpty), err)
	}

	// 2. EnqueueTriggerRequestStructs with empty slice
	err = q.EnqueueTriggerRequestStructs(nil)
	if err != nil {
		t.Errorf("expected nil err on empty slice, got %v", err)
	}

	// 3. EnqueueTriggerRequestStructs with requests
	reqs := []JobTriggerRequest{
		{JobID: "SCH-peek-1", ReloadRetries: 1},
		{JobID: "SCH-peek-2", ReloadRetries: 0},
	}
	err = q.EnqueueTriggerRequestStructs(reqs)
	if err != nil {
		t.Fatalf("failed to enqueue request structs: %v", err)
	}

	// 4. PeekTriggerRequests after enqueueing
	peekPopulated, err := q.PeekTriggerRequests()
	if err != nil {
		t.Fatalf("peek error: %v", err)
	}
	if len(peekPopulated) != 2 {
		t.Errorf("expected 2 peeked requests, got %d", len(peekPopulated))
	}
	if peekPopulated[0].JobID != "SCH-peek-1" || peekPopulated[0].ReloadRetries != 1 {
		t.Errorf("unexpected request contents: %+v", peekPopulated[0])
	}
}

func TestExtended_CapDispatch_VerificationAndMint(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-cap-disp-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("storage failed: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. mintAgentTaskID
	atkID := mintAgentTaskID()
	if !strings.HasPrefix(atkID, "ATK-") {
		t.Errorf("expected ATK- prefix, got %s", atkID)
	}

	// 2. resumePendingVerificationTasks with empty planID
	err = h.resumePendingVerificationTasks(ctx, "zqk", "")
	if err != nil {
		t.Errorf("expected nil err for empty planID, got %v", err)
	}

	// 3. resumePendingVerificationTasks with task missing commit evidence
	planID := "PRI-TEST-VERIF-1"
	taskNoCommitsID := "ATK-1785886324283087000-nocommit1"
	bgCtx := pkgctx.WithLifecycleBreakGlass(ctx, "test-pending-verif")
	_ = sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:              taskNoCommitsID,
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:          objects.ObjectStatusPendingVerification,
		objects.FieldKeyPriorityPlanRef: planID,
	})
	listRes, listErr := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storagepkg.ListFilter{
		Kind: objects.KindAgentTask,
		Filters: map[string]any{
			objects.FieldKeyStatus:          objects.ObjectStatusPendingVerification,
			objects.FieldKeyPriorityPlanRef: planID,
		},
	})
	t.Logf("direct List: len=%d, err=%v", len(listRes.Objects), listErr)

	err = h.resumePendingVerificationTasks(ctx, "zqk", planID)
	t.Logf("resumePendingVerificationTasks err: %v", err)

	// 4. resumePendingVerificationTasks with task having commit hashes (not merged into branch)
	taskWithCommitsID := "ATK-1785886324283087000-commits02"
	crErr2 := sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:              taskWithCommitsID,
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:          objects.ObjectStatusPendingVerification,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyCommitHashes:     []any{"deadbeef1234"},
	})
	t.Logf("taskWithCommits create: %v", crErr2)

	// Delete the no-commits task so it doesn't block this test
	_ = sp.Delete(ctx, secCtx, taskNoCommitsID, false)

	// Since deadbeef1234 is not merged, allMerged=false and wakeAgentAndScheduleHourglass is called
	err = h.resumePendingVerificationTasks(ctx, "zqk", planID)
	if err != nil {
		t.Errorf("expected nil err when unmerged commits trigger wake, got %v", err)
	}
}
