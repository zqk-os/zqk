package system

import (
	"fmt"
	"time"

	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/maintenance"
	"github.com/spf13/cobra"
)

func NewCleanBranchesCmd() *cobra.Command {
	var inactiveHours int
	var quarantineDays int
	var targets []string

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCleanBranchesCommandBuilder(), &cobra.Command{
		Use:   "clean-branches",
		Short: "Prune stale merged feature branches locally and remotely",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			svc := maintenance.NewGitMaintenanceService(".")

			dur := time.Duration(inactiveHours) * time.Hour
			logger := logging.GetLoggerFromContext(ctx)
			logging.FluentEvent(logger).Info(fmt.Sprintf("Running git cleanup sweep. Targets: %v, Inactive: %vh, Quarantine Retention: %vd", targets, inactiveHours, quarantineDays)).Log()

			count, err := svc.PruneStaleBranches(ctx, dur, targets)
			if err != nil {
				cmd.PrintErrln("Error in PruneStaleBranches:", err)
				return err
			}

			cmd.Printf("Successfully pruned %d stale branches.\n", count)

			// Purge ancient quarantined refs
			qDur := time.Duration(quarantineDays) * 24 * time.Hour
			qCount, qErr := svc.PruneQuarantinedRefs(ctx, qDur)
			if qErr != nil {
				cmd.PrintErrln("Error in PruneQuarantinedRefs:", qErr)
			} else if qCount > 0 {
				cmd.Printf("Successfully pruned %d expired quarantined refs.\n", qCount)
			}

			return nil
		},
	})

	cmd.Flags().IntVar(&inactiveHours, "inactive-hours", 24, "Minimum hours of inactivity required before deletion")
	cmd.Flags().IntVar(&quarantineDays, "quarantine-days", 90, "Retention limit in days for soft-deleted quarantine refs/archive/ branches")
	cmd.Flags().StringSliceVar(&targets, "targets", []string{"origin/main", "origin/integration/v1.1", "origin/integration/v1.2", "origin/integration/v1.3", "origin/integration/v1.4"}, "Target branches to check against")

	return cmd
}
