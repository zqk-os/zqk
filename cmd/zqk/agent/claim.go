package agent

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentclaim"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func NewClaimCmd() *cobra.Command {
	var claimant string
	var forRef string
	var cvsRef string
	var exitWhenCVSCompleted bool
	var hourglassOn bool
	var checkinCadence time.Duration

	cmd := &cobra.Command{
		Use:   "claim [task_id]",
		Short: "Atomically claim an agent_task for exclusive execution",
		Long:  "Sets claimed_by and claimed_at for exclusive multi-agent execution. Fails if another agent already holds the claim.",
		Args:  cobra.ExactArgs(1),
		RunE: cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			ctx := proc.OperationContext()
			sec := proc.SecurityContext()
			if sec == nil {
				sec = pkgctx.NewSystemSecurityContext()
			}
			who := strings.TrimSpace(claimant)
			if who == "" {
				who = resolveClaimantIdentity(cmd, proc)
			}

			// Extract extra options
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
		}),
	}
	cmd.Flags().StringVar(&claimant, "by", "", "Claimant agent/account id (default: seating/env identity)")
	cmd.Flags().StringVar(&forRef, "for", "", "Target BLI or related object reference")
	cmd.Flags().StringVar(&cvsRef, "cvs", "", "Active CVS reference")
	cmd.Flags().BoolVar(&exitWhenCVSCompleted, "exit-when-cvs-completed", false, "Block exit until CVS is complete")
	cmd.Flags().BoolVar(&hourglassOn, "hourglass-on", false, "Enable hourglass deadline wake signal")
	cmd.Flags().DurationVar(&checkinCadence, "checkin-cadence", agentclaim.DefaultCheckinCadence,
		"How long the claim may stay silent before the orchestrator is woken")
	cli.AddCommonFlags(cmd)
	return cmd
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

// NewReleaseCmd creates `zqk agent release <ATK-id>`.
// TRACK: BLI-1785886173393325000-d0690a02
func NewReleaseCmd() *cobra.Command {
	var claimant string
	var force bool
	cmd := &cobra.Command{
		Use:   "release [task_id]",
		Short: "Release an agent_task execution claim",
		Long:  "Clears claimed_by/claimed_at when held by --by (or --force).",
		Args:  cobra.ExactArgs(1),
		RunE: cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			ctx := proc.OperationContext()
			sec := proc.SecurityContext()
			if sec == nil {
				sec = pkgctx.NewSystemSecurityContext()
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
		}),
	}
	cmd.Flags().StringVar(&claimant, "by", "", "Claimant that must hold the claim")
	cmd.Flags().BoolVar(&force, "force", false, "Clear claim regardless of holder")
	cli.AddCommonFlags(cmd)
	return cmd
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
