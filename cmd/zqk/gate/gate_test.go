package gate

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clictx "github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/gate"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func executeGate(t *testing.T, projectRoot string, args ...string) (string, error) {
	t.Helper()
	cmd := NewGateCmd()
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cliCtx := &cli.Context{
		Context: &clictx.Context{
			ProjectRoot: projectRoot,
			Profile:     "debug",
		},
	}
	cli.SetContext(cmd, cliCtx)
	for _, sub := range cmd.Commands() {
		cli.SetContext(sub, cliCtx)
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestNewGateCmd_Structure(t *testing.T) {
	cmd := NewGateCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "gate", cmd.Use)

	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}

	assert.True(t, subcommands["stage"], "missing stage subcommand")
	assert.True(t, subcommands["review"], "missing review subcommand")
	assert.True(t, subcommands["approve"], "missing approve subcommand")
	assert.True(t, subcommands["reject"], "missing reject subcommand")
	assert.True(t, subcommands["list"], "missing list subcommand")
}

func TestGate_EndToEndLifecycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// Create a payload file using fileutil
	payloadFile := filepath.Join(proj.Root, "mutation_payload.json")
	payloadContent := []byte(`{"action": "grant_permission", "role": "admin"}`)
	err := fileutil.WriteStandardFile(payloadFile, payloadContent)
	require.NoError(t, err)

	// 1. Stage approval request
	out, err := executeGate(t, proj.Root, "stage", "--type", "security_grant", "--target", "iam-service", "--payload", payloadFile, "--created-by", "dev-alice")
	require.NoError(t, err, "stage failed: %s", out)
	assert.Contains(t, out, "Approval request staged successfully")
	assert.Contains(t, out, "APR-")

	// Extract approval ID from output
	var approvalID string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "ID:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				approvalID = strings.TrimSpace(parts[1])
				break
			}
		}
	}
	require.NotEmpty(t, approvalID, "approval ID should be found in output: %s", out)

	// 2. Review staged approval request
	reviewOut, err := executeGate(t, proj.Root, "review", approvalID)
	require.NoError(t, err, "review failed: %s", reviewOut)
	assert.Contains(t, reviewOut, approvalID)
	assert.Contains(t, reviewOut, "security_grant")
	assert.Contains(t, reviewOut, "iam-service")
	assert.Contains(t, reviewOut, "grant_permission")

	// Verify gate verification fails while staged
	verified, err := gate.VerifyApproval(context.Background(), proj.FileStorage, approvalID, payloadContent)
	assert.Error(t, err)
	assert.False(t, verified)

	// 3. Approve request
	approveOut, err := executeGate(t, proj.Root, "approve", approvalID, "--reviewer", "security-lead")
	require.NoError(t, err, "approve failed: %s", approveOut)
	assert.Contains(t, approveOut, "Approval successfully granted")
	assert.Contains(t, approveOut, "security-lead")

	// 4. Verify gate verification passes now that it is approved
	verified, err = gate.VerifyApproval(context.Background(), proj.FileStorage, approvalID, payloadContent)
	require.NoError(t, err)
	assert.True(t, verified)

	// 5. Verify gate verification fails if payload is tampered
	verified, err = gate.VerifyApproval(context.Background(), proj.FileStorage, approvalID, []byte(`{"action": "malicious"}`))
	assert.Error(t, err)
	assert.False(t, verified)

	// 6. Test list command
	listOut, err := executeGate(t, proj.Root, "list")
	require.NoError(t, err, "list failed: %s", listOut)
	assert.Contains(t, listOut, approvalID)
}

func TestGate_RejectLifecycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	payloadFile := filepath.Join(proj.Root, "bad_payload.json")
	payloadContent := []byte(`{"delete_all": true}`)
	err := fileutil.WriteStandardFile(payloadFile, payloadContent)
	require.NoError(t, err)

	// Stage
	out, err := executeGate(t, proj.Root, "stage", "--type", "drop_db", "--target", "db-cluster", "--payload", payloadFile, "--json")
	require.NoError(t, err, "stage failed: %s", out)
	assert.Contains(t, out, "APR-")

	var staged gate.ApprovalRequest
	err = json.Unmarshal([]byte(out), &staged)
	require.NoError(t, err)

	// Reject
	rejectOut, err := executeGate(t, proj.Root, "reject", staged.ID, "--reviewer", "operator-bob", "--reason", "Destructive operations prohibited", "--json")
	require.NoError(t, err, "reject failed: %s", rejectOut)
	assert.Contains(t, rejectOut, "rejected")

	// Review should show rejected status and reason
	reviewOut, err := executeGate(t, proj.Root, "review", staged.ID)
	require.NoError(t, err)
	assert.Contains(t, reviewOut, "rejected")
	assert.Contains(t, reviewOut, "Destructive operations prohibited")

	// Approve must fail on already rejected request
	approveOut, err := executeGate(t, proj.Root, "approve", staged.ID, "--reviewer", "operator-charlie")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot approve rejected request")
	_ = approveOut
}
