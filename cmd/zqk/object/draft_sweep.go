package object

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewDraftSweepCmd creates `object draft sweep`.
// POL-AGENT-DRAFT-SWEEP-TPM-001
func NewDraftSweepCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectDraftSweepCommandBuilder()
	cmd.Flags().Bool("force", false, "Force sweep in local developer workspaces without ACC account")
	cli.BindAsyncProgress(cmd, runObjectDraftSweep)
	return cmd
}

func runObjectDraftSweep(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		flags, err := parseDraftPlaneCLIFlags(cmd)
		if err != nil {
			return err
		}
		agentID, err := cmd.Flags().GetString("agent-id")
		if err != nil {
			return err
		}
		_ = agentID // optional audit/seat correlation only — auth is RBAC SecurityContext
		force, _ := cmd.Flags().GetBool("force")
		if !flags.dryRun {
			if authErr := authorizeDraftSweepApply(proc.SecurityContext(), force); authErr != nil {
				return authErr
			}
		}
		result, sweepErr := storage.SweepObjectDraftPlane(proc.ProjectRoot(), storage.ObjectDraftPlaneSweepOptions{
			Kind:      flags.matchOpts.Kind,
			IDPrefix:  flags.matchOpts.IDPrefix,
			Status:    flags.matchOpts.Status,
			OlderThan: flags.matchOpts.OlderThan,
			DryRun:    flags.dryRun,
			All:       flags.matchOpts.All,
			Max:       flags.matchOpts.Max,
		})
		if sweepErr != nil {
			return sweepErr
		}
		return cli.FormatOutput(cmd, result)
	})(cmd, args)
}
