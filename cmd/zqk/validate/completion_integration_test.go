package validate

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

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

	// draft-plane create / promote membrane.
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
	// draft-plane create / promote membrane.
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

	// Execute NewVerifyCompletionCmd on backlog_item
	cmdBli := NewVerifyCompletionCmd()
	var bufBli bytes.Buffer
	cmdBli.SetOut(&bufBli)
	cmdBli.SetErr(&bufBli)
	baseCtxBli := pkgctx.WithCommandOutputWriter(context.Background(), &bufBli)
	baseCtxBli = pkgctx.WithSecurityContext(baseCtxBli, secCtx)
	cmdBli.SetContext(cli.WithStorageProvider(baseCtxBli, fs))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctxBli, err := cli.GetContextFromCommand(cmdBli, initCtx)
	require.NoError(t, err)
	cctxBli.ProjectRoot = proj.Root
	cli.SetContext(cmdBli, cctxBli)
	cmdBli.SetArgs([]string{bliID})
	err = cmdBli.Execute()
	assert.NoError(t, err)

	// Execute NewVerifyCompletionCmd on test_case
	cmdTc := NewVerifyCompletionCmd()
	var bufTc bytes.Buffer
	cmdTc.SetOut(&bufTc)
	cmdTc.SetErr(&bufTc)
	baseCtxTc := pkgctx.WithCommandOutputWriter(context.Background(), &bufTc)
	baseCtxTc = pkgctx.WithSecurityContext(baseCtxTc, secCtx)
	cmdTc.SetContext(cli.WithStorageProvider(baseCtxTc, fs))
	cctxTc, err := cli.GetContextFromCommand(cmdTc, initCtx)
	require.NoError(t, err)
	cctxTc.ProjectRoot = proj.Root
	cli.SetContext(cmdTc, cctxTc)
	cmdTc.SetArgs([]string{tcID})
	err = cmdTc.Execute()
	assert.NoError(t, err)

	// Execute NewVerifyCompletionCmd on non-existent object
	cmdErr := NewVerifyCompletionCmd()
	var bufErr bytes.Buffer
	cmdErr.SetOut(&bufErr)
	cmdErr.SetErr(&bufErr)
	baseCtxErr := pkgctx.WithCommandOutputWriter(context.Background(), &bufErr)
	baseCtxErr = pkgctx.WithSecurityContext(baseCtxErr, secCtx)
	cmdErr.SetContext(cli.WithStorageProvider(baseCtxErr, fs))
	cctxErr, err := cli.GetContextFromCommand(cmdErr, initCtx)
	require.NoError(t, err)
	cctxErr.ProjectRoot = proj.Root
	cli.SetContext(cmdErr, cctxErr)
	cmdErr.SetArgs([]string{"DOES-NOT-EXIST"})
	err = cmdErr.Execute()
	assert.Error(t, err)

	// Execute NewVerifyCompletionCmd on unsupported kind (goal)
	cmdGoal := NewVerifyCompletionCmd()
	var bufGoal bytes.Buffer
	cmdGoal.SetOut(&bufGoal)
	cmdGoal.SetErr(&bufGoal)
	baseCtxGoal := pkgctx.WithCommandOutputWriter(context.Background(), &bufGoal)
	baseCtxGoal = pkgctx.WithSecurityContext(baseCtxGoal, secCtx)
	cmdGoal.SetContext(cli.WithStorageProvider(baseCtxGoal, fs))
	cctxGoal, err := cli.GetContextFromCommand(cmdGoal, initCtx)
	require.NoError(t, err)
	cctxGoal.ProjectRoot = proj.Root
	cli.SetContext(cmdGoal, cctxGoal)
	cmdGoal.SetArgs([]string{goalID})
	err = cmdGoal.Execute()
	assert.Error(t, err)
}
