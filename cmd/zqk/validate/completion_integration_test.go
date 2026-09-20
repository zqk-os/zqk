package validate

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"
)

func TestVerifyCompletionIntegration(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "cmd.validate.verify_completion",
		SeedSchemaPlane:          true,
		ForceRemoveRootOnCleanup: true,
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
	goalObj := map[string]any{
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeyID:            goalID,
		objects.FieldKeyTitle:         "Integration goal",
		objects.FieldKeyTarget:        "1",
		objects.FieldKeyMetric:        "count",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	acID := "CRIT-1"
	critObj := map[string]any{
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyID:            acID,
		objects.FieldKeyTitle:         "Integration criteria",
		objects.FieldKeyStatus:        "awaiting_verification",
		objects.FieldKeyCategory:      "acceptance",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	bliObj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              bliID,
		objects.FieldKeyTitle:           "Integration backlog item",
		objects.FieldKeyStatus:          objects.ObjectStatusExploring,
		objects.FieldKeyCategory:        "development",
		objects.FieldKeyGoalRefs:        []string{goalID},
		objects.FieldKeyCriteriaRefs:    []string{acID},
		objects.FieldKeyPriorityPlanRef: "PRI-1",
		objects.FieldKeyMilestoneRefs:   []string{"MILE-1"},
		objects.FieldKeyPriorityTier:    "P1",
		objects.FieldKeyEstimatedEffort: "4h",
		objects.FieldKeyCommitHashes:    []any{productCommit},
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
	}

	reqID := "REQ-1"
	reqObj := map[string]any{
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyID:            reqID,
		objects.FieldKeyTitle:         "Integration requirement",
		objects.FieldKeyDescription:   "Verify completion hashes propagate to linked objects.",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeyPriority:      "p1",
		objects.FieldKeyGoalRefs:      []string{goalID},
		objects.FieldKeyCriteriaRefs:  []string{acID},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	storage.CreateCASVisible(t, fs, ctx, secCtx, goalObj, objects.ObjectStatusActive)
	storage.CreateCASVisible(t, fs, ctx, secCtx, critObj, objects.ObjectStatusInProgress)
	critRead, err := fs.Read(ctx, secCtx, acID)
	require.NoError(t, err)
	critRead[objects.FieldKeyStatus] = objects.ObjectStatusInProgress
	require.NoError(t, fs.Update(ctx, secCtx, acID, critRead))
	critRead[objects.FieldKeyStatus] = objects.ObjectStatusValidated
	require.NoError(t, fs.Update(ctx, secCtx, acID, critRead))
	storage.CreateCASVisible(t, fs, ctx, secCtx, bliObj, objects.ObjectStatusPlanned)
	storage.CreateCASVisible(t, fs, ctx, secCtx, reqObj, objects.ObjectStatusActive)

	tcID := "TEST-1"
	tcObj := map[string]any{
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
	}
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	storage.CreateCASVisible(t, fs, ctx, secCtx, tcObj, objects.ObjectStatusActive)

	err = testkit.SignTestCaseCompletion(ctx, tcID, []string{"src/dummy.go"})
	require.NoError(t, err)

	handler := scheduler.NewAutoTransitionHandler(fs)
	tcRead, err := fs.Read(ctx, secCtx, tcID)
	require.NoError(t, err)

	tcRead[objects.FieldKeyStatus] = "complete"
	if _, ok := tcRead[objects.FieldKeyVerificationHash]; !ok {
		tcRead[objects.FieldKeyVerificationHash] = "dummy-hash"
	}
	_ = fs.Update(ctx, secCtx, tcID, tcRead)

	evtCtx := scheduler.WithEventData(ctx, tcRead)
	job := &scheduler.ScheduledJob{ID: "job-1"}

	err = handler.Execute(evtCtx, job)
	require.NoError(t, err)

	bliRead, err := fs.Read(ctx, secCtx, bliID)
	require.NoError(t, err)
	bliHash, _ := bliRead[objects.FieldKeyVerificationHash].(string)
	assert.NotEmpty(t, bliHash, "backlog_item should have verification_hash")

	reqRead, err := fs.Read(ctx, secCtx, reqID)
	require.NoError(t, err)
	reqHash, _ := reqRead[objects.FieldKeyVerificationHash].(string)
	assert.NotEmpty(t, reqHash, "requirement should have verification_hash")
}
