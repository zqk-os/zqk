package system_test

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"
)

func TestVerifyCompletionIntegration(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	fs := proj.FileStorage

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	specsDir := filepath.Join(proj.Root, paths.ProcessInternalObjectSpecsDir)
	require.NoError(t, fileutil.EnsureDir(specsDir))
	generator := builders.NewSpecGenerator(specsDir)
	require.NoError(t, generator.GenerateAllSpecs())

	artifactPath := filepath.Join(proj.Root, "src", "dummy.go")
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(artifactPath)))
	require.NoError(t, fileutil.WriteStandardFile(artifactPath, []byte("package dummy\n")))

	goalID := "GOAL-1"
	err := fs.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyKind:   "goal",
		objects.FieldKeyID:     goalID,
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	})
	require.NoError(t, err)

	acID := "CRIT-1"
	err = fs.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyKind:     "criteria",
		objects.FieldKeyID:       acID,
		objects.FieldKeyStatus:   objects.ObjectStatusInProgress,
		objects.FieldKeyCategory: "acceptance",
	})
	require.NoError(t, err)

	bliID := "ITEM-1"
	err = fs.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyKind:         "backlog_item",
		objects.FieldKeyID:           bliID,
		objects.FieldKeyStatus:       objects.ObjectStatusInProgress,
		objects.FieldKeyGoalRefs:     []string{goalID},
		objects.FieldKeyCriteriaRefs: []string{acID},
	})
	require.NoError(t, err)

	reqID := "REQ-1"
	err = fs.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyKind:            "requirement",
		objects.FieldKeyID:              reqID,
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyBacklogItemRefs: []string{bliID},
		objects.FieldKeyGoalRefs:        []string{goalID},
		objects.FieldKeyCriteriaRefs:    []string{acID},
	})
	require.NoError(t, err)

	tcID := "TEST-1"
	err = fs.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyKind:            "test_case",
		objects.FieldKeyID:              tcID,
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyBacklogItemRefs: []string{bliID},
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyGoalRefs:        []string{goalID},
		objects.FieldKeyCriteriaRefs:    []string{acID},
	})
	require.NoError(t, err)

	err = testkit.SignTestCaseCompletion(ctx, tcID, []string{"src/dummy.go"})
	require.NoError(t, err)

	handler := scheduler.NewAutoTransitionHandler(fs)
	tcObj, err := fs.Read(ctx, secCtx, tcID)
	require.NoError(t, err)

	tcObj[objects.FieldKeyStatus] = "complete"
	if _, ok := tcObj[objects.FieldKeyVerificationHash]; !ok {
		tcObj[objects.FieldKeyVerificationHash] = "dummy-hash"
	}
	fs.Update(ctx, secCtx, tcID, tcObj)

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
