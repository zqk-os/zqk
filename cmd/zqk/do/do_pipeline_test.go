package do

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func createDoTestCommand(t *testing.T, projectRoot string, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := NewDoCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	baseCtx := pkgctx.NewSystemContext()
	baseCtx = pkgctx.WithCommandOutputWriter(baseCtx, &buf)
	if secCtx != nil {
		baseCtx = pkgctx.WithSecurityContext(baseCtx, secCtx)
	}
	cmd.SetContext(cli.WithStorageProvider(baseCtx, storageProvider))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	ctx.ProjectRoot = projectRoot
	cli.SetContext(cmd, ctx)
	cmd.SetArgs(args)

	return cmd, &buf
}

func TestRunDo_DryRunAndSummary(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Create a planned backlog item in storage
	bli := map[string]any{
		objects.FieldKeyKind:        objects.KindBacklogItem,
		objects.FieldKeyID:          "BLI-DO-TEST-001",
		objects.FieldKeyTitle:       "Test Autonomous Do Item",
		objects.FieldKeyDescription: "Item to exercise zqk do pipeline",
		objects.FieldKeyStatus:      objects.ObjectStatusPlanned,
	}
	require.NoError(t, p.FileStorage.Create(ctx, secCtx, bli))

	// 1. Execute dry run with specific target
	cmd1, buf1 := createDoTestCommand(t, p.Root, p.FileStorage, secCtx, "BLI-DO-TEST-001", "--dry-run")
	err := cmd1.Execute()
	require.NoError(t, err)
	out1 := buf1.String()
	assert.Contains(t, out1, "BLI-DO-TEST-001")
	assert.Contains(t, out1, "Execution Steps")
	assert.Contains(t, out1, "Next Steps for You & Your AI Agent")

	// 2. Execute dry run in JSON format
	cmd2, buf2 := createDoTestCommand(t, p.Root, p.FileStorage, secCtx, "BLI-DO-TEST-001", "--dry-run", "-f", "json")
	err = cmd2.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf2.String(), `"target_bli_id": "BLI-DO-TEST-001"`)

	// 3. Execute dry run with custom claimant via --by
	cmd3, buf3 := createDoTestCommand(t, p.Root, p.FileStorage, secCtx, "BLI-DO-TEST-001", "--dry-run", "--by", "seat-custom-worker")
	err = cmd3.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf3.String(), "seat-custom-worker")

	// 4. Execute dry run with ZQKENV_AGENT_ID set
	t.Setenv("ZQK_AGENT_ID", "agent-env-worker")
	cmd4, buf4 := createDoTestCommand(t, p.Root, p.FileStorage, secCtx, "BLI-DO-TEST-001", "--dry-run")
	err = cmd4.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf4.String(), "agent-env-worker")
}

func TestRunDo_TargetNotFound(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	secCtx := pkgctx.NewSystemSecurityContext()

	cmd, buf := createDoTestCommand(t, p.Root, p.FileStorage, secCtx, "BLI-NONEXISTENT")
	assert.NotNil(t, buf)
	err := cmd.Execute()
	require.Error(t, err)
}

func TestRunDo_NoPlannedWork(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	secCtx := pkgctx.NewSystemSecurityContext()

	cmd, buf := createDoTestCommand(t, p.Root, p.FileStorage, secCtx)
	assert.NotNil(t, buf)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no eligible backlog items")
}

func TestRenderDoSummary_FailedAndTaskVariants(t *testing.T) {
	cmd := NewDoCmd()
	var buf strings.Builder
	cmd.SetOut(&buf)

	// Summary for a task target with a failed step
	res := &clipkg.AutoExecResult{
		TargetTaskID: "TSK-001",
		Status:       "failed",
		Claimant:     "agent:auto-exec",
		Steps: []clipkg.AutoExecStepResult{
			{Step: "discovery", StepStatus: "ok", Detail: "Resolved task"},
			{Step: "verification", StepStatus: "failed", Detail: "Test failure encountered"},
		},
	}

	renderDoSummary(cmd, res)
	out := buf.String()

	assert.Contains(t, out, "TSK-001")
	assert.Contains(t, out, "❌")
	assert.Contains(t, out, "Test failure encountered")
}
