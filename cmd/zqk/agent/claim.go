package agent

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentclaim"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func NewClaimCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentClaimCommandBuilder()
	cmd.RunE = cli.WithProcessor(runClaim)
	return cmd
}

func runClaim(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	var flags clipkg.FlagBag
	claimant := flags.String(cmd, "by")
	forRef := flags.String(cmd, "for")
	cvsRef := flags.String(cmd, "cvs")
	exitWhenCVSCompleted := flags.Bool(cmd, "exit-when-cvs-completed")
	hourglassOn := flags.Bool(cmd, "hourglass-on")
	checkinCadence := flags.Duration(cmd, "checkin-cadence")
	if err := flags.Err(); err != nil {
		return err
	}
	who := strings.TrimSpace(claimant)
	if who == "" {
		who = resolveClaimantIdentity(cmd, proc)
	}

	opts := agentclaim.ClaimOptions{
		ForRef:               forRef,
		CVSRef:               cvsRef,
		ExitWhenCVSCompleted: exitWhenCVSCompleted,
		HourglassOn:          hourglassOn,
		ProjectRoot:          proc.ProjectRoot(),
		CheckinCadence:       checkinCadence,
	}

	res, err := agentclaim.TryClaim(ctx, proc.Storage(), sec, args[0], who, opts)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"task_id":                 args[0],
		"claimed":                 res.Claimed,
		objects.FieldKeyClaimedBy: res.ClaimedBy,
		objects.FieldKeyClaimedAt: res.ClaimedAt,
		objects.FieldKeyReason:    res.Reason,
		"checkin_due":             checkinDue(proc.ProjectRoot(), args[0]),
	}
	return cli.FormatOutput(cmd, payload)
}

// checkinDue reports the check-in deadline so the claimant learns the cadence at claim
// time rather than discovering it when the orchestrator asks why they went quiet.
func checkinDue(projectRoot, taskID string) string {
	timer, err := agentclaim.LoadCheckin(projectRoot, taskID)
	if err != nil || timer == nil {
		return ""
	}
	return timer.ExpiresAt
}

func NewReleaseCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentReleaseCommandBuilder()
	cmd.RunE = cli.WithProcessor(runRelease)
	return cmd
}

func runRelease(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	var flags clipkg.FlagBag
	claimant := flags.String(cmd, "by")
	force := flags.Bool(cmd, "force")
	if err := flags.Err(); err != nil {
		return err
	}
	who := strings.TrimSpace(claimant)
	if who == "" && !force {
		who = resolveClaimantIdentity(cmd, proc)
	}
	res, err := agentclaim.Release(ctx, proc.Storage(), sec, args[0], who, force, proc.ProjectRoot())
	if err != nil {
		return err
	}
	payload := map[string]any{
		"task_id":              args[0],
		"released":             res.Released,
		objects.FieldKeyReason: res.Reason,
	}
	return cli.FormatOutput(cmd, payload)
}

func resolveClaimantIdentity(cmd *cobra.Command, proc *cli.Processor) string {
	if v := strings.TrimSpace(zqkenv.AgentID().Get()); v != "" {
		return v
	}
	if proc != nil && proc.SecurityContext() != nil {
		if acc := strings.TrimSpace(proc.SecurityContext().AccountID); acc != "" {
			return acc
		}
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		return "host:" + host
	}
	_ = cmd
	return "anonymous"
}

// claimTaskForExecute applies work-claim before swarm execute.
func claimTaskForExecute(cmd *cobra.Command, proc *cli.Processor, taskID string, task map[string]any) error {
	if task == nil {
		return nil
	}
	kind, _ := task[objects.FieldKeyKind].(string)
	if kind != objects.KindAgentTask {
		return nil
	}
	who := resolveClaimantIdentity(cmd, proc)
	sec := proc.SecurityContext()
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	_, err := agentclaim.TryClaim(proc.OperationContext(), proc.Storage(), sec, taskID, who)
	if err != nil {
		return errfmt.Newf("work claim failed for %s", taskID).Wrap(err)
	}
	return nil
}

// releaseTaskAfterNext clears claim when worker advances via agent next.
func releaseTaskAfterNext(proc *cli.Processor, taskID, claimant string) error {
	sec := proc.SecurityContext()
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	who := strings.TrimSpace(claimant)
	if who == "" {
		who = resolveClaimantIdentity(nil, proc)
	}
	_, err := agentclaim.Release(proc.OperationContext(), proc.Storage(), sec, taskID, who, true, proc.ProjectRoot())
	return err
}
