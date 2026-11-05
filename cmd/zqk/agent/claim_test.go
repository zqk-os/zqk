package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/cmd/zqk/agent"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestClaimCommand_Flags(t *testing.T) {
	root, store := setupOrchestrateTest(t)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-test-123"
	persona := map[string]any{
		objects.FieldKeyID:          "PER-TEST",
		objects.FieldKeyKind:        objects.KindPersona,
		objects.FieldKeyTitle:       "Test Persona",
		objects.FieldKeyName:        "Test Persona",
		objects.FieldKeyRole:        "agent",
		objects.FieldKeyStatus:      objects.ObjectStatusImplemented,
		objects.FieldKeyDescription: "Test agent persona for claiming tasks in orchestrate.",
	}
	err := store.Create(ctx, sec, persona)
	assert.NoError(t, err)

	cvs := map[string]any{
		objects.FieldKeyID:     "CVS-test-456",
		objects.FieldKeyKind:   objects.KindConvergenceSession,
		objects.FieldKeyTitle:  "Test CVS",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	err = store.Create(ctx, sec, cvs)
	assert.NoError(t, err)

	bli := map[string]any{
		objects.FieldKeyID:          "BLI-test-123",
		objects.FieldKeyKind:        objects.KindBacklogItem,
		objects.FieldKeyTitle:       "Test BLI",
		objects.FieldKeyStatus:      objects.ObjectStatusPlanned,
		objects.FieldKeyDescription: "Test backlog item description for claiming tasks.",
	}
	err = store.Create(ctx, sec, bli)
	assert.NoError(t, err)

	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Test Task",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyDescription:        "Test agent task description for claiming.",
		objects.FieldKeyAssigneePersonaRef: "PER-TEST",
	}
	err = store.Create(ctx, sec, task)
	assert.NoError(t, err)

	// Flush storage so CLI reads it
	flushCtx, flushCancel := storage.DurabilityFlushContext()
	err = storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, store, root, []string{
		objects.KindAgentTask, objects.KindPersona, objects.KindConvergenceSession, objects.KindBacklogItem,
	})
	flushCancel()
	assert.NoError(t, err)

	cmd := agent.NewClaimCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))

	cmd.SetArgs([]string{
		taskID,
		"--by", "test-agent",
		"--for", "BLI-test-123",
		"--cvs", "CVS-test-456",
		"--exit-when-cvs-completed",
		"--hourglass-on",
	})

	err = cmd.Execute()
	assert.NoError(t, err)

	// Verify object state
	cliStore, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	assert.NoError(t, err)
	obj, err := cliStore.Read(ctx, sec, taskID)
	assert.NoError(t, err)

	assert.Equal(t, "test-agent", obj[objects.FieldKeyClaimedBy])
	assert.Equal(t, "BLI-test-123", obj["for_ref"])
	assert.Equal(t, "CVS-test-456", obj["cvs_ref"])
	assert.Equal(t, true, obj["exit_when_cvs_completed"])
	assert.Equal(t, true, obj["hourglass_on"])
}

func TestReleaseCommand_BuilderFlags(t *testing.T) {
	cmd := agent.NewReleaseCmd()
	assert.NotNil(t, cmd.Flags().Lookup("by"))
	assert.NotNil(t, cmd.Flags().Lookup("agent"))
	assert.NotNil(t, cmd.Flags().Lookup("agent-id"))
	assert.NotNil(t, cmd.Flags().Lookup("force"))
	assert.Nil(t, cmd.Flags().Lookup("hourglass-on"))
}

func TestClaimCommand_AliasesAndEstimatedEffort(t *testing.T) {
	root, store := setupOrchestrateTest(t)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	persona := map[string]any{
		objects.FieldKeyID:          "PER-TEST",
		objects.FieldKeyKind:        objects.KindPersona,
		objects.FieldKeyTitle:       "Test Persona",
		objects.FieldKeyName:        "Test Persona",
		objects.FieldKeyRole:        "agent",
		objects.FieldKeyStatus:      objects.ObjectStatusImplemented,
		objects.FieldKeyDescription: "Test agent persona for claiming tasks in orchestrate.",
	}
	assert.NoError(t, store.Create(ctx, sec, persona))

	taskID1 := "ATK-alias-1"
	task1 := map[string]any{
		objects.FieldKeyID:                 taskID1,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Test Task Alias 1",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyDescription:        "Test agent task description without estimated effort.",
		objects.FieldKeyAssigneePersonaRef: "PER-TEST",
	}
	assert.NoError(t, store.Create(ctx, sec, task1))

	taskID2 := "ATK-alias-2"
	task2 := map[string]any{
		objects.FieldKeyID:                 taskID2,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Test Task Alias 2",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyDescription:        "Test agent task description with explicit effort.",
		objects.FieldKeyAssigneePersonaRef: "PER-TEST",
	}
	assert.NoError(t, store.Create(ctx, sec, task2))

	taskID3 := "ATK-alias-3"
	task3 := map[string]any{
		objects.FieldKeyID:                 taskID3,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Test Task Alias 3",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyDescription:        "Test agent task description with effort alias.",
		objects.FieldKeyAssigneePersonaRef: "PER-TEST",
	}
	assert.NoError(t, store.Create(ctx, sec, task3))

	flushCtx, flushCancel := storage.DurabilityFlushContext()
	assert.NoError(t, storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, store, root, []string{
		objects.KindAgentTask, objects.KindPersona,
	}))
	flushCancel()

	// 1. Claim task1 using --agent (alias for --by) without --estimated-effort (defaults to "2h")
	cmd1 := agent.NewClaimCmd()
	cli.SetContext(cmd1, cli.ContextForProjectRoot(root))
	cmd1.SetArgs([]string{taskID1, "--agent", "agent-foo"})
	assert.NoError(t, cmd1.Execute())

	// 2. Claim task2 using --agent-id (alias for --by) and --estimated-effort "4h"
	cmd2 := agent.NewClaimCmd()
	cli.SetContext(cmd2, cli.ContextForProjectRoot(root))
	cmd2.SetArgs([]string{taskID2, "--agent-id", "agent-bar", "--estimated-effort", "4h"})
	assert.NoError(t, cmd2.Execute())

	// 3. Claim task3 using --agent and --effort "30m"
	cmd3 := agent.NewClaimCmd()
	cli.SetContext(cmd3, cli.ContextForProjectRoot(root))
	cmd3.SetArgs([]string{taskID3, "--agent", "agent-baz", "--effort", "30m"})
	assert.NoError(t, cmd3.Execute())

	cliStore, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	assert.NoError(t, err)

	obj1, err := cliStore.Read(ctx, sec, taskID1)
	assert.NoError(t, err)
	assert.Equal(t, "agent-foo", obj1[objects.FieldKeyClaimedBy])
	assert.Equal(t, "2h", obj1[objects.FieldKeyEstimatedEffort])

	obj2, err := cliStore.Read(ctx, sec, taskID2)
	assert.NoError(t, err)
	assert.Equal(t, "agent-bar", obj2[objects.FieldKeyClaimedBy])
	assert.Equal(t, "4h", obj2[objects.FieldKeyEstimatedEffort])

	obj3, err := cliStore.Read(ctx, sec, taskID3)
	assert.NoError(t, err)
	assert.Equal(t, "agent-baz", obj3[objects.FieldKeyClaimedBy])
	assert.Equal(t, "30m", obj3[objects.FieldKeyEstimatedEffort])
}
