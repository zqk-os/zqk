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
	assert.NotNil(t, cmd.Flags().Lookup("force"))
	assert.Nil(t, cmd.Flags().Lookup("hourglass-on"))
}
