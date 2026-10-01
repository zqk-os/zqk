package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentclaim"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	flagClaimGateBy                = "by"
	flagClaimGateAssignment        = "assignment"
	flagClaimGateAutoAssign        = "auto-assign"
	flagClaimGateWake              = "wake"
	flagClaimGateRequireAssignment = "require-assignment"
)

func NewClaimGateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentClaimGateCommandBuilder()
	cmd.RunE = cli.WithProcessor(runAgentClaimGate)
	return cmd
}

func runAgentClaimGate(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
	var flags clipkg.FlagBag
	claimant := strings.TrimSpace(flags.String(cmd, flagClaimGateBy))
	assignment := strings.TrimSpace(flags.String(cmd, flagClaimGateAssignment))
	autoAssign := flags.Bool(cmd, flagClaimGateAutoAssign)
	wake := flags.Bool(cmd, flagClaimGateWake)
	requireAssignment := flags.Bool(cmd, flagClaimGateRequireAssignment)
	if err := flags.Err(); err != nil {
		return err
	}
	if claimant == "" {
		claimant = resolveClaimantIdentity(cmd, proc)
	}
	if assignment == "" {
		assignment = activeIntentAssignment(proc.ProjectRoot())
	}

	ctx := proc.OperationContext()
	sec := proc.SecurityContext()
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}

	opts := agentclaim.GateOptions{
		Assignment:        assignment,
		RequireAssignment: requireAssignment,
		EmptyPoolFallback: func() (string, bool, error) {
			return agentclaim.ReclaimOrMintFallback(ctx, proc.Storage(), sec, claimant, proc.ProjectRoot())
		},
	}
	if autoAssign {
		opts.Mode = agentclaim.GateModeAutoAssign
		opts.AutoClaim = func() (string, error) {
			return agentclaim.ClaimNextOccupiable(ctx, proc.Storage(), sec, claimant, assignment, proc.ProjectRoot())
		}
	}

	dec, err := agentclaim.GateWrite(proc.ProjectRoot(), claimant, opts)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"allowed":              dec.Allowed,
		objects.FieldKeyReason: dec.Reason,
		"claimant":             dec.Claimant,
		"task_ids":             dec.TaskIDs,
		"auto_assigned_id":     dec.AutoAssignedID,
		"assignment":           assignment,
		"wake":                 wake && dec.AutoAssignedID != "",
		"interrupt":            dec.Message,
	}
	if fmtErr := cli.FormatOutput(cmd, payload); fmtErr != nil {
		return fmtErr
	}
	if dec.Allowed {
		return nil
	}
	return dec.Electrocute(cmd.Context())
}

func activeIntentAssignment(projectRoot string) string {
	pointer := filepath.Join(paths.StateDirPath(projectRoot), "change_intent_active")
	raw, err := fileutil.ReadFile(pointer)
	if err != nil {
		return ""
	}
	rel := strings.TrimSpace(string(raw))
	if rel == "" {
		return ""
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectRoot, rel)
	}
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return ""
	}
	var intent map[string]any
	if json.Unmarshal(data, &intent) != nil {
		return ""
	}
	s, _ := intent["assignment"].(string)
	return strings.TrimSpace(s)
}
