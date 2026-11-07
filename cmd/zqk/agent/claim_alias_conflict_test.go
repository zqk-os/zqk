package agent_test

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/cmd/zqk/agent"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

const (
	gh932Persona   = "PER-GH932"
	gh932TaskAgree = "ATK-gh932-agree"
	gh932Seat      = "seat-gh932"
	gh932OtherSeat = "seat-other"
)

// GH-932: --agent / --agent-id are spec-declared aliases with independent values
// so conflicting inputs can be detected (fail closed) instead of last-wins.
func TestClaimAliasFlags_DeclaredInSpecBuilder(t *testing.T) {
	for _, mk := range []func() *cobra.Command{agent.NewClaimCmd, agent.NewReleaseCmd} {
		cmd := mk()
		by := cmd.Flags().Lookup("by")
		require.NotNil(t, by)
		for _, alias := range []string{"agent", "agent-id"} {
			f := cmd.Flags().Lookup(alias)
			require.NotNil(t, f, "%s missing --%s", cmd.Name(), alias)
			assert.NotSame(t, by.Value, f.Value, "--%s must not share --by storage", alias)
		}
	}
}

func TestClaimAliasFlags_ConflictingValuesFailClosed(t *testing.T) {
	cases := [][]string{
		{"--by", gh932Seat, "--agent", gh932OtherSeat},
		{"--by", gh932Seat, "--agent-id", gh932OtherSeat},
		{"--agent", gh932Seat, "--agent-id", gh932OtherSeat},
	}
	for _, mk := range []func() *cobra.Command{agent.NewClaimCmd, agent.NewReleaseCmd} {
		for _, flagArgs := range cases {
			cmd := mk()
			root := t.TempDir()
			cli.SetContext(cmd, cli.ContextForProjectRoot(root))
			cmd.SetArgs(append([]string{"ATK-gh932-conflict"}, flagArgs...))
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			err := cmd.Execute()
			require.Error(t, err, "%s %v", cmd.Name(), flagArgs)
			assert.Contains(t, err.Error(), "conflicting claimant flags")
		}
	}
}

func TestClaimAliasFlags_IdenticalValuesAccepted(t *testing.T) {
	root, store := setupOrchestrateTest(t)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	require.NoError(t, store.Create(ctx, sec, map[string]any{
		objects.FieldKeyID:          gh932Persona,
		objects.FieldKeyKind:        objects.KindPersona,
		objects.FieldKeyTitle:       "GH-932 Persona",
		objects.FieldKeyName:        "GH-932 Persona",
		objects.FieldKeyRole:        "agent",
		objects.FieldKeyStatus:      objects.ObjectStatusImplemented,
		objects.FieldKeyDescription: "Persona for GH-932 claimant alias agreement test.",
	}))
	require.NoError(t, store.Create(ctx, sec, map[string]any{
		objects.FieldKeyID:                 gh932TaskAgree,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "GH-932 agreeing aliases",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyDescription:        "Claim with --by, --agent and --agent-id all agreeing.",
		objects.FieldKeyAssigneePersonaRef: gh932Persona,
	}))
	flushCtx, flushCancel := storage.DurabilityFlushContext()
	require.NoError(t, storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, store, root, []string{
		objects.KindAgentTask, objects.KindPersona,
	}))
	flushCancel()

	cmd := agent.NewClaimCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	cmd.SetArgs([]string{gh932TaskAgree, "--by", gh932Seat, "--agent", gh932Seat, "--agent-id", " " + gh932Seat + " "})
	require.NoError(t, cmd.Execute())

	cliStore, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	require.NoError(t, err)
	obj, err := cliStore.Read(ctx, sec, gh932TaskAgree)
	require.NoError(t, err)
	assert.Equal(t, gh932Seat, obj[objects.FieldKeyClaimedBy])
}
