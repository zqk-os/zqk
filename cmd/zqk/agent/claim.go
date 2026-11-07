package agent

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/agentclaim"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Claimant flag names: --by is canonical; --agent and --agent-id are spec-declared
// aliases (GH-932) matching the vocabulary of the rest of the `zqk agent` family.
const (
	claimantFlagBy      = "by"
	claimantFlagAgent   = "agent"
	claimantFlagAgentID = "agent-id"
)

// claimantFlagNames is the resolution order for the claimant identity flags.
var claimantFlagNames = []string{claimantFlagBy, claimantFlagAgent, claimantFlagAgentID}

func NewClaimCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentClaimCommandBuilder()
	if cmd.Flags().Lookup("estimated-effort") == nil {
		cmd.Flags().String("estimated-effort", "", "Estimated effort duration (e.g. 2h, 30m; required when effort-aware objects enter in_progress)")
	}
	if cmd.Flags().Lookup("effort") == nil {
		cmd.Flags().String("effort", "", "Alias for --estimated-effort")
	}
	cmd.RunE = cli.WithProcessor(runClaim)
	return cmd
}

// parseClaimant resolves the claimant from --by / --agent / --agent-id. Supplying
// more than one is allowed only when all non-empty values agree; conflicting values
// fail closed rather than silently picking one.
func parseClaimant(cmd *cobra.Command, proc *cli.Processor, flags *clipkg.FlagBag, fallbackIfEmpty bool) (string, error) {
	claimant, source := "", ""
	for _, name := range claimantFlagNames {
		if cmd == nil || cmd.Flags().Lookup(name) == nil {
			continue
		}
		v := strings.TrimSpace(flags.String(cmd, name))
		if v == "" {
			continue
		}
		if claimant == "" {
			claimant, source = v, name
			continue
		}
		if v != claimant {
			return "", errfmt.Errorf("conflicting claimant flags: --%s=%q and --%s=%q (--agent and --agent-id are aliases for --by; pass one value)", source, claimant, name, v)
		}
	}
	if claimant == "" && fallbackIfEmpty {
		return resolveClaimantIdentity(cmd, proc), nil
	}
	return claimant, nil
}

func runClaim(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	var flags clipkg.FlagBag
	who, err := parseClaimant(cmd, proc, &flags, true)
	if err != nil {
		return err
	}
	forRef := flags.String(cmd, "for")
	cvsRef := flags.String(cmd, "cvs")
	exitWhenCVSCompleted := flags.Bool(cmd, "exit-when-cvs-completed")
	hourglassOn := flags.Bool(cmd, "hourglass-on")
	checkinCadence := flags.Duration(cmd, "checkin-cadence")
	var estimatedEffort string
	if cmd != nil && cmd.Flags().Lookup("estimated-effort") != nil {
		estimatedEffort = flags.String(cmd, "estimated-effort")
	}
	if strings.TrimSpace(estimatedEffort) == "" && cmd != nil && cmd.Flags().Lookup("effort") != nil {
		estimatedEffort = flags.String(cmd, "effort")
	}
	if err := flags.Err(); err != nil {
		return err
	}

	opts := agentclaim.ClaimOptions{
		ForRef:               forRef,
		CVSRef:               cvsRef,
		ExitWhenCVSCompleted: exitWhenCVSCompleted,
		HourglassOn:          hourglassOn,
		ProjectRoot:          proc.ProjectRoot(),
		CheckinCadence:       checkinCadence,
		EstimatedEffort:      strings.TrimSpace(estimatedEffort),
	}

	ctx, sec, sp := procStorageTuple(proc)
	res, err := agentclaim.TryClaim(ctx, sp, sec, args[0], who, opts)
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
	var flags clipkg.FlagBag
	force := flags.Bool(cmd, "force")
	who, err := parseClaimant(cmd, proc, &flags, !force)
	if err != nil {
		return err
	}
	if err := flags.Err(); err != nil {
		return err
	}
	ctx, sec, sp := procStorageTuple(proc)
	res, err := agentclaim.Release(ctx, sp, sec, args[0], who, force, proc.ProjectRoot())
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
	ctx, sec, sp := procStorageTuple(proc)
	_, err := agentclaim.TryClaim(ctx, sp, sec, taskID, who)
	if err != nil {
		return errfmt.Newf("work claim failed for %s", taskID).Wrap(err)
	}
	return nil
}

// releaseTaskAfterNext clears claim when worker advances via agent next.
func releaseTaskAfterNext(proc *cli.Processor, taskID, claimant string) error {
	ctx, sec, sp := procStorageTuple(proc)
	who := strings.TrimSpace(claimant)
	if who == "" {
		who = resolveClaimantIdentity(nil, proc)
	}
	_, err := agentclaim.Release(ctx, sp, sec, taskID, who, true, proc.ProjectRoot())
	return err
}
