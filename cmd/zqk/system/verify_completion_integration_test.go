package system_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"
)

func TestVerifyCompletionIntegration(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "cmd.system.verify_completion",
		SeedSchemaPlane: true,
	})
	fs := proj.FileStorage

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	artifactPath := filepath.Join(proj.Root, "src", "dummy.go")
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(artifactPath)))
	require.NoError(t, fileutil.WriteStandardFile(artifactPath, []byte("package dummy\n")))
	bliID := "BLI-1"
	productCommit := testkit.ProductCommitMentioningBacklog(t, proj.Root, bliID, "src/dummy.go")

	goalID := "GOAL-1"
	storage.CreateCASVisible(t, fs, ctx, secCtx, map[string]any{
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyID:     goalID,
		objects.FieldKeyTitle:  "Integration goal",
		objects.FieldKeyStatus: objects.ObjectStatusProposed,
	}, objects.ObjectStatusActive)

	acID := "CRIT-1"
	storage.CreateCASVisible(t, fs, ctx, secCtx, map[string]any{
		objects.FieldKeyKind:     objects.KindCriteria,
		objects.FieldKeyID:       acID,
		objects.FieldKeyTitle:    "Integration criteria",
		objects.FieldKeyStatus:   "awaiting_verification",
		objects.FieldKeyCategory: "acceptance",
	}, objects.ObjectStatusInProgress)
	critObj, err := fs.Read(ctx, secCtx, acID)
	require.NoError(t, err)
	critObj[objects.FieldKeyStatus] = objects.ObjectStatusInProgress
	require.NoError(t, fs.Update(ctx, secCtx, acID, critObj))
	critObj[objects.FieldKeyStatus] = objects.ObjectStatusValidated
	require.NoError(t, fs.Update(ctx, secCtx, acID, critObj))

	storage.CreateCASVisible(t, fs, ctx, secCtx, map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              bliID,
		objects.FieldKeyTitle:           "Integration backlog",
		objects.FieldKeyStatus:          objects.ObjectStatusExploring,
		objects.FieldKeyGoalRefs:        []string{goalID},
		objects.FieldKeyCriteriaRefs:    []string{acID},
		objects.FieldKeyPriorityPlanRef: "PRI-1",
		objects.FieldKeyMilestoneRefs:   []string{"MILE-1"},
		objects.FieldKeyPriorityTier:    "P1",
		objects.FieldKeyEstimatedEffort: "4h",
		objects.FieldKeyCommitHashes:    []any{productCommit},
	}, objects.ObjectStatusPlanned)

	reqID := "REQ-1"
	storage.CreateCASVisible(t, fs, ctx, secCtx, map[string]any{
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyID:           reqID,
		objects.FieldKeyTitle:        "Integration requirement",
		objects.FieldKeyDescription:  "Verify completion hashes propagate to linked objects.",
		objects.FieldKeyStatus:       objects.ObjectStatusProposed,
		objects.FieldKeyPriority:     "p1",
		objects.FieldKeyGoalRefs:     []string{goalID},
		objects.FieldKeyCriteriaRefs: []string{acID},
	}, objects.ObjectStatusActive)

	tcID := "TEST-1"
	storage.CreateCASVisible(t, fs, ctx, secCtx, map[string]any{
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyID:              tcID,
		objects.FieldKeyTitle:           "Integration test case",
		objects.FieldKeyStatus:          objects.ObjectStatusDraft,
		objects.FieldKeyPathOrID:        "src/dummy.go",
		objects.FieldKeyScope:           "integration",
		objects.FieldKeyBacklogItemRefs: []string{bliID},
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyGoalRefs:        []string{goalID},
		objects.FieldKeyCriteriaRefs:    []string{acID},
	}, objects.ObjectStatusActive)
	storage.FlushAllOrFail(t, proj.Root)

	err = testkit.SignTestCaseCompletion(ctx, tcID, []string{"src/dummy.go"})
	require.NoError(t, err)

	handler := scheduler.NewAutoTransitionHandler(fs)
	tcObj, err := fs.Read(ctx, secCtx, tcID)
	require.NoError(t, err)

	tcObj[objects.FieldKeyStatus] = "complete"
	if _, ok := tcObj[objects.FieldKeyVerificationHash]; !ok {
		tcObj[objects.FieldKeyVerificationHash] = "dummy-hash"
	}
	_ = fs.Update(ctx, secCtx, tcID, tcObj)

	evtCtx := scheduler.WithEventData(ctx, tcObj)
	job := &scheduler.ScheduledJob{ID: "job-1"}

	err = handler.Execute(evtCtx, job)
	require.NoError(t, err)

	bliObj, err := fs.Read(ctx, secCtx, bliID)
	require.NoError(t, err)
	bliHash, _ := bliObj[objects.FieldKeyVerificationHash].(string)
	assert.NotEmpty(t, bliHash, "backlog_item should have verification_hash")

	reqObj, err := fs.Read(ctx, secCtx, reqID)
	require.NoError(t, err)
	reqHash, _ := reqObj[objects.FieldKeyVerificationHash].(string)
	assert.NotEmpty(t, reqHash, "requirement should have verification_hash")
}
