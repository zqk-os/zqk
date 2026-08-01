package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileObjectStorage_DispatchStatusGateway(t *testing.T) {
	testRoot, storage, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	ctx := context.Background()
	processDir := datacell.ProcessPrimaryDir(testRoot)

	// Setup necessary directories for our test objects
	routerDir := filepath.Join(processDir, "shockwave_routers")
	require.NoError(t, os.MkdirAll(routerDir, paths.DirPerm755))

	taskDir := filepath.Join(processDir, "agent_tasks")
	require.NoError(t, os.MkdirAll(taskDir, paths.DirPerm755))

	milestoneDir := filepath.Join(processDir, "milestones")
	require.NoError(t, os.MkdirAll(milestoneDir, paths.DirPerm755))

	// 0. Create a persona
	personaDir := filepath.Join(processDir, "personas")
	require.NoError(t, os.MkdirAll(personaDir, paths.DirPerm755))
	personaObj := map[string]any{
		objects.FieldKeyID:    "PER-001",
		objects.FieldKeyKind:  "persona",
		objects.FieldKeyTitle: "Test Persona",
	}
	err := storage.Create(ctx, secCtx, personaObj)
	require.NoError(t, err, "Failed to create persona")

	// 1. Create a parent object (milestone) in status 'planned'
	parentObj := map[string]any{
		objects.FieldKeyID:     "MIL-GATEWAY-TEST-001",
		objects.FieldKeyKind:   "milestone",
		objects.FieldKeyStatus: "planned",
		objects.FieldKeyTitle:  "Parent Milestone for Gateway Test",
	}
	err = storage.Create(ctx, secCtx, parentObj)
	require.NoError(t, err, "Failed to create parent milestone")

	// 2. Create a child object (agent_task) in status 'planned'
	childObj := map[string]any{
		objects.FieldKeyID:                 "ATK-GATEWAY-TEST-001",
		objects.FieldKeyKind:               "agent_task",
		objects.FieldKeyStatus:             "planned",
		objects.FieldKeyTitle:              "Child Task for Gateway Test",
		"milestone_ref":                    "MIL-GATEWAY-TEST-001",
		objects.FieldKeyAssigneePersonaRef: "PER-001",
	}
	err = storage.Create(ctx, secCtx, childObj)
	require.NoError(t, err, "Failed to create child task")

	// 3. Create the active shockwave_router object
	routerObj := map[string]any{
		objects.FieldKeyID:     "SWR-STATUS-GATEWAY-TEST",
		objects.FieldKeyKind:   "shockwave_router",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyTitle:  "Status Auto-Bubble Gateway for Tests",
		objects.FieldKeyTriggerCondition: map[string]any{
			objects.FieldKeyKind: "agent_task",
		},
		objects.FieldKeyPropagationRules: map[string]any{},
	}
	err = storage.Create(ctx, secCtx, routerObj)
	require.NoError(t, err, "Failed to create shockwave router")

	// 4. Update the child task's status to 'in_progress'
	updates := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}
	err = storage.Update(ctx, secCtx, "ATK-GATEWAY-TEST-001", updates)
	require.NoError(t, err, "Failed to update child task status")

	// 5. Verify the parent milestone was forcefully updated to 'in_progress' by the GatewayDispatcher!
	updatedParent, err := storage.Read(ctx, secCtx, "MIL-GATEWAY-TEST-001")
	require.NoError(t, err, "Failed to read updated parent milestone")

	parentStatus, ok := updatedParent[objects.FieldKeyStatus].(string)
	require.True(t, ok, "Parent status should be a string")
	assert.Equal(t, objects.ObjectStatusInProgress, parentStatus, "GatewayDispatcher should have bubbled the status to the parent milestone")

	// 6. Test fail path: If parent cannot be updated (e.g. missing required metrics), the gateway blocks the child.
	// Since we are not strictly enforcing metrics in this simple unit test environment, we just test the propagation.
}
