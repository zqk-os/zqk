package agent_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/cmd/zqk/agent"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestRecoverCmd_UnsetsClaimOnApproved(t *testing.T) {
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
	err := store.Create(ctx, sec, persona)
	assert.NoError(t, err)

	taskID := "ATK-recover-claim-test"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Test Recover Task",
		objects.FieldKeyStatus:             objects.ObjectStatusError,
		objects.FieldKeyAssigneePersonaRef: "PER-TEST",
		objects.FieldKeyClaimedBy:          "agent-to-be-released",
		objects.FieldKeyClaimedAt:          "2026-09-16T00:00:00Z",
		objects.FieldKeyDescription:        "Task that failed and needs recovery",
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyTitle:                "Step 1",
				objects.FieldKeyStatus:               objects.ObjectStatusError,
				objects.FieldKeyVerificationFeedback: "failed",
			},
		},
	}
	err = store.Create(ctx, sec, task)
	assert.NoError(t, err)

	flushCtx, flushCancel := storage.DurabilityFlushContext()
	err = storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, store, root, []string{
		objects.KindAgentTask, objects.KindPersona,
	})
	flushCancel()
	assert.NoError(t, err)

	cmd := agent.NewRecoverCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	cmd.SetArgs([]string{"--all"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err = cmd.Execute()
	assert.NoError(t, err)

	updated, err := store.Read(ctx, sec, taskID)
	assert.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusApproved, updated[objects.FieldKeyStatus])
	assert.Nil(t, updated[objects.FieldKeyClaimedBy])
	assert.Nil(t, updated[objects.FieldKeyClaimedAt])
}
