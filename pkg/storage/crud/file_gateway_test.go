package crud_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFileObjectStorage_DispatchStatusGatewayDoesNotWalkParents(t *testing.T) {
	testRoot, fos, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	_ = testRoot
	ctx := context.Background()
	processDir := datacell.ProcessPrimaryDir(testRoot)

	require.NoError(t, fileutil.MkdirAll(filepath.Join(processDir, "milestones"), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(processDir, "agent_tasks"), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(processDir, "personas"), paths.DirPerm755))

	storage.CreateCASVisible(t, fos, ctx, secCtx, map[string]any{
		objects.FieldKeyID:     "PER-001",
		objects.FieldKeyKind:   "persona",
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
		objects.FieldKeyTitle:  "Test Persona",
		objects.FieldKeyName:   "Test Persona",
		objects.FieldKeyRole:   "tester",
	}, objects.ObjectStatusApproved)
	storage.CreateCASVisible(t, fos, ctx, secCtx, map[string]any{
		objects.FieldKeyID:              "MIL-GATEWAY-TEST-001",
		objects.FieldKeyKind:            "milestone",
		objects.FieldKeyStatus:          objects.ObjectStatusNotStarted,
		objects.FieldKeyTitle:           "Parent Milestone for Gateway Test",
		objects.FieldKeyEstimatedEffort: "1w",
	}, objects.ObjectStatusNotStarted)
	storage.CreateCASVisible(t, fos, ctx, secCtx, map[string]any{
		objects.FieldKeyID:                 "ATK-GATEWAY-TEST-001",
		objects.FieldKeyKind:               "agent_task",
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyTitle:              "Child Task for Gateway Test",
		"milestone_ref":                    "MIL-GATEWAY-TEST-001",
		objects.FieldKeyAssigneePersonaRef: "PER-001",
	}, "approved")

	require.NoError(t, fos.Update(ctx, secCtx, "ATK-GATEWAY-TEST-001", map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}))

	updatedParent, err := fos.Read(ctx, secCtx, "MIL-GATEWAY-TEST-001")
	require.NoError(t, err)
	parentStatus, ok := updatedParent[objects.FieldKeyStatus].(string)
	require.True(t, ok)
	assert.Equal(t, objects.ObjectStatusNotStarted, parentStatus, "storage gateway must not walk parents")
}

func TestFileObjectStorage_StatusReactiveListener_MilestoneRefs(t *testing.T) {
	testRoot, fos, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	ctx := context.Background()
	processDir := datacell.ProcessPrimaryDir(testRoot)

	require.NoError(t, fileutil.MkdirAll(filepath.Join(processDir, "milestones"), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(processDir, "backlog"), paths.DirPerm755))

	storage.SetLifecycleHookHandler(func(ctx context.Context, kind, from, to string, objectData map[string]any) error {
		id, _ := objectData[objects.FieldKeyID].(string)
		lifecycle.ApplyDependencyRefEvents(ctx, logging.NewEventLogger(ctx), fos, testRoot, kind, id, from, to, objectData)
		return nil
	})
	t.Cleanup(func() { storage.SetLifecycleHookHandler(nil) })

	storage.CreateCASVisible(t, fos, ctx, secCtx, map[string]any{
		objects.FieldKeyID:              "MIL-GATEWAY-REFS-001",
		objects.FieldKeyKind:            objects.KindMilestone,
		objects.FieldKeyStatus:          objects.ObjectStatusNotStarted,
		objects.FieldKeyTitle:           "Parent Milestone via milestone_refs",
		objects.FieldKeyEstimatedEffort: "1w",
	}, objects.ObjectStatusNotStarted)

	storage.CreateCASVisible(t, fos, ctx, secCtx, map[string]any{
		objects.FieldKeyID:              "BLI-GATEWAY-REFS-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyEstimatedEffort: "1d",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyTitle:           "Child BLI for milestone_refs listener",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-GATEWAY-REFS-001"},
	}, objects.ObjectStatusPlanned)

	bg := pkgctx.WithLifecycleBreakGlass(ctx, "status_reactive milestone_refs hop")
	bg = pkgctx.WithAllowCoreObjectDelete(bg)
	require.NoError(t, fos.Update(bg, secCtx, "BLI-GATEWAY-REFS-001", map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}))

	updatedParent, err := fos.Read(ctx, secCtx, "MIL-GATEWAY-REFS-001")
	require.NoError(t, err)
	parentStatus, ok := updatedParent[objects.FieldKeyStatus].(string)
	require.True(t, ok)
	assert.Equal(t, objects.ObjectStatusInProgress, parentStatus, "status_reactive milestone must react to BLI in_progress")
}

func TestFileObjectStorage_StatusReactiveListener_CriteriaRefs(t *testing.T) {
	testRoot, fos, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	ctx := context.Background()
	processDir := datacell.ProcessPrimaryDir(testRoot)

	require.NoError(t, fileutil.MkdirAll(filepath.Join(processDir, "criteria"), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(processDir, "backlog"), paths.DirPerm755))

	storage.SetLifecycleHookHandler(func(ctx context.Context, kind, from, to string, objectData map[string]any) error {
		id, _ := objectData[objects.FieldKeyID].(string)
		lifecycle.ApplyDependencyRefEvents(ctx, logging.NewEventLogger(ctx), fos, testRoot, kind, id, from, to, objectData)
		return nil
	})
	t.Cleanup(func() { storage.SetLifecycleHookHandler(nil) })

	storage.CreateCASVisible(t, fos, ctx, secCtx, map[string]any{
		objects.FieldKeyID:       "CRIT-GATEWAY-LOCK-001",
		objects.FieldKeyKind:     objects.KindCriteria,
		objects.FieldKeyStatus:   objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyTitle:    "Shovel-ready criterion for parent lock",
		objects.FieldKeyCategory: "acceptance",
	}, objects.ObjectStatusAwaitingVerification)

	storage.CreateCASVisible(t, fos, ctx, secCtx, map[string]any{
		objects.FieldKeyID:              "BLI-GATEWAY-CRIT-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyEstimatedEffort: "1d",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyTitle:           "Child BLI for criteria_refs listener",
		objects.FieldKeyCriteriaRefs:    []any{"CRIT-GATEWAY-LOCK-001"},
	}, objects.ObjectStatusPlanned)

	bg := pkgctx.WithLifecycleBreakGlass(ctx, "status_reactive criteria_refs hop")
	bg = pkgctx.WithAllowCoreObjectDelete(bg)
	require.NoError(t, fos.Update(bg, secCtx, "BLI-GATEWAY-CRIT-001", map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}))

	updated, err := fos.Read(ctx, secCtx, "CRIT-GATEWAY-LOCK-001")
	require.NoError(t, err)
	got, ok := updated[objects.FieldKeyStatus].(string)
	require.True(t, ok)
	assert.Equal(t, objects.ObjectStatusInProgress, got, "status_reactive criteria must lock when linked BLI enters in_progress")
	assert.NotEqual(t, objects.ObjectStatusValidated, got, "parent-lock must not land on validated")
}
