package scheduler

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

func NewStateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerStateCommandBuilder()
	// Same orchestration as scheduler status: local PID/state files only; avoid scheduler
	// check + session paths that can block or contend with the running daemon.
	cli.RequireSchedulerCheck(cmd, false)
	cli.RequireSession(cmd, false)
	cmd.RunE = runState
	return cmd
}

func runState(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveCommandProjectRoot(cmd)
	if err != nil {
		return err
	}
	migrate, _ := cmd.Flags().GetBool("migrate")
	if migrate {
		reg := schedulerpkg.NewJobStateRegistry(projectRoot)
		nFlat, err := reg.MigrateLegacyFlatStateFilesBestEffort()
		if err != nil {
			return errfmt.Newf("migrate scheduler state").Wrap(err)
		}
		nBucket, err := reg.MigrateUnbucketedJobStateDirsBestEffort()
		if err != nil {
			return errfmt.Newf("migrate scheduler state buckets").Wrap(err)
		}
		msg := fmt.Sprintf("Migrated %d legacy flat state file(s); moved %d job state directories into workload buckets.\n", nFlat, nBucket)
		return cli.WriteOutput(cmd, []byte(msg))
	}
	staleAfter, _ := cmd.Flags().GetDuration("stale-after")
	reg := schedulerpkg.NewJobStateRegistry(projectRoot)
	summary, err := reg.Summarize(staleAfter)
	if err != nil {
		return errfmt.Newf("summarize scheduler state").Wrap(err)
	}
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, summary)
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Scheduler state summary\n")
	_, _ = fmt.Fprintf(&b, "  total files: %d\n", summary.TotalFiles)
	_, _ = fmt.Fprintf(&b, "  stale in_progress/deferred (> %ds): %d\n", summary.StaleThresholdSec, summary.StaleInProgress)
	_, _ = fmt.Fprintf(&b, "  by state:\n")
	for _, s := range []string{schedulerStateInProgress, schedulerStateDeferred, schedulerStateCompleted, schedulerStateFailed, schedulerStateSkipped} {
		_, _ = fmt.Fprintf(&b, "    %s: %d\n", s, summary.ByState[s])
	}
	_, _ = fmt.Fprintf(&b, "  hint: see .zqk/scheduler/state/_README.yaml\n")
	return cli.WriteOutput(cmd, []byte(b.String()))
}
