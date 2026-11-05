package swarm

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	pkgswarm "github.com/zqk-os/zqk/pkg/swarm"
)

// NewHeartbeatCmd creates the swarm heartbeat command.
func NewHeartbeatCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSwarmHeartbeatCommandBuilder(), &cobra.Command{
		RunE: runHeartbeat,
	})
	cli.AddCommonFlags(cmd)
	return cmd
}

func runHeartbeat(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		cadence, _ := cmd.Flags().GetDuration("cadence")
		once, _ := cmd.Flags().GetBool("once")
		ctx := proc.OperationContext()
		sec := proc.ResolvedSecurityContext()
		if sec == nil {
			sec = pkgctx.NewSystemSecurityContext()
		}
		sp := proc.Storage()
		root := proc.ProjectRoot()
		if root == "" {
			root = paths.ResolveProjectRoot(".")
		}

		opts := pkgswarm.HeartbeatOptions{
			ProjectRoot:     root,
			Cadence:         cadence,
			Once:            once,
			Storage:         sp,
			SecurityContext: sec,
		}

		out := cmd.OutOrStdout()
		onPulse := func(report *pkgswarm.HeartbeatReport) {
			if cli.GetFormat(cmd) == cli.FormatJSON || cli.GetFormat(cmd) == cli.FormatYAML {
				_ = cli.FormatOutput(cmd, report)
				return
			}
			fmt.Fprintf(out, "💓 [SWARM HEARTBEAT] Cadence Pulse completed at %s (interval: %s)\n",
				report.Timestamp.Format("15:04:05"), report.Cadence)
			fmt.Fprintf(out, "   🧹 Hygiene Sweep: %d stale lock(s) cleared, %d temp file(s) cleaned, membrane healthy: %v\n",
				report.Hygiene.StaleLocksCleared, report.Hygiene.TempFilesCleaned, report.Hygiene.MembraneHealthy)
			if len(report.Hygiene.OrphanAlerts) > 0 {
				alertCount := len(report.Hygiene.OrphanAlerts)
				displayLimit := alertCount
				if displayLimit > 3 {
					displayLimit = 3
				}
				fmt.Fprintf(out, "      ⚠️ Orphan alerts (%d): %s\n",
					alertCount, strings.Join(report.Hygiene.OrphanAlerts[:displayLimit], "; "))
			}
			fmt.Fprintf(out, "   🎯 Alignment Pulse: score %.1f%% (%d goals, %d aligned, %d gaps) -> %s\n",
				report.Alignment.AlignmentScore, report.Alignment.GoalsCount, report.Alignment.ItemsWithGoals,
				report.Alignment.ItemsWithoutGoals, report.Alignment.CachePath)
			fmt.Fprintf(out, "   ⚡ Reactive Wakeup: dispatched: %v (unblocked work: %d, pending approvals: %d)\n",
				report.Wakeup.WakeupDispatched, report.Wakeup.UnblockedWorkCount, report.Wakeup.PendingApprovalCount)
		}

		if err := pkgswarm.RunHeartbeatLoop(ctx, opts, onPulse); err != nil {
			return errfmt.Newf("swarm heartbeat failed").Wrap(err)
		}

		return nil
	})(cmd, args)
}
