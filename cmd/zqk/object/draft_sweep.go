package object

import (
	"fmt"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewDraftSweepCmd creates `object draft sweep`.
// TRACK: BLI-1785827957031623000-b08b9791 / POL-AGENT-DRAFT-SWEEP-TPM-001
func NewDraftSweepCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectDraftSweepCommandBuilder()
	cli.BindAsyncProgress(cmd, runObjectDraftSweep)
	return cmd
}

func runObjectDraftSweep(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}
		all, err := cmd.Flags().GetBool("all")
		if err != nil {
			return err
		}
		kind, err := cmd.Flags().GetString("kind")
		if err != nil {
			return err
		}
		idPrefix, err := cmd.Flags().GetString("id-prefix")
		if err != nil {
			return err
		}
		status, err := cmd.Flags().GetString("status")
		if err != nil {
			return err
		}
		olderThanStr, err := cmd.Flags().GetString("older-than")
		if err != nil {
			return err
		}
		maxN, err := cmd.Flags().GetInt("max")
		if err != nil {
			return err
		}
		agentID, err := cmd.Flags().GetString("agent-id")
		if err != nil {
			return err
		}
		_ = agentID // optional audit/seat correlation only — auth is RBAC SecurityContext
		if !dryRun {
			if authErr := authorizeDraftSweepApply(proc.SecurityContext()); authErr != nil {
				return authErr
			}
		}
		var olderThan time.Duration
		if olderThanStr != "" {
			olderThan, err = time.ParseDuration(olderThanStr)
			if err != nil {
				return fmt.Errorf("invalid --older-than %q: %w", olderThanStr, err)
			}
		}
		result, sweepErr := storage.SweepObjectDraftPlane(proc.ProjectRoot(), storage.ObjectDraftPlaneSweepOptions{
			Kind:      kind,
			IDPrefix:  idPrefix,
			Status:    status,
			OlderThan: olderThan,
			DryRun:    dryRun,
			All:       all,
			Max:       maxN,
		})
		if sweepErr != nil {
			return sweepErr
		}
		return cli.FormatOutput(cmd, result)
	})(cmd, args)
}
