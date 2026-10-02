package scheduler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewEventsCmd creates the events parent command (events health).
// Aggregation runs automatically via the SCH-evag timer job; no manual aggregate subcommand.
func NewEventsCmd() *cobra.Command {
	eventsCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerEventsCommandBuilder(), &cobra.Command{
		Use:   "events",
		Short: "Scheduler events (health view over metrics summary)",
	})
	healthCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerHealthCommandBuilder(), &cobra.Command{
		Use:   "health",
		Short: "Show health view over scheduler-metrics-summary (failures and slow jobs)",
		Long:  "Reads .zqk/scheduler/scheduler-metrics-summary.json and lists jobs with failures or average duration above threshold (e.g. > 5 min). The summary is updated automatically by the SCH-evag timer job; to run aggregation on demand, trigger that job.",
	})
	healthCmd.RunE = runEventsHealth
	eventsCmd.AddCommand(healthCmd)
	return eventsCmd
}

func runEventsHealth(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveCommandProjectRoot(cmd)
	if err != nil {
		return err
	}

	summaryPath := schedpkg.SummaryPath(projectRoot)
	data, err := fileutil.ReadFile(summaryPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return cli.WriteOutput(cmd, []byte(fmt.Sprintf(
				"No summary found at %s. The SCH-evag timer job updates it every 15 min; trigger that job for on-demand aggregation.\n",
				summaryPath,
			)))
		}
		return errfmt.Newf("read summary").Wrap(err)
	}

	var summary schedpkg.SchedulerMetricsSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return errfmt.Newf("parse summary").Wrap(err)
	}
	if summary.JobStats == nil {
		summary.JobStats = make(map[string]schedpkg.JobExecutionStats)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Health view (window_end=%s, slow_job_threshold_sec=%d)\n", summary.WindowEndISO, schedpkg.SlowJobThresholdSec)
	fmt.Fprintf(&b, "Jobs with failures or avg duration > %ds:\n", schedpkg.SlowJobThresholdSec)

	anyUnhealthy := false
	for jobID, st := range summary.JobStats {
		runs := st.Completed + st.Failed
		if runs == 0 {
			continue
		}
		avgSec := st.TotalDurationSec / float64(runs)
		hasFailures := st.Failed > 0
		slow := avgSec > float64(schedpkg.SlowJobThresholdSec)
		if !hasFailures && !slow {
			continue
		}
		anyUnhealthy = true
		flags := ""
		if hasFailures {
			flags = "has_failures"
		}
		if slow {
			if flags != emptyValue {
				flags += ",slow"
			} else {
				flags = "slow"
			}
		}
		fmt.Fprintf(&b, "  %s  completed=%d failed=%d avg_duration_sec=%.1f  [%s]", jobID, st.Completed, st.Failed, avgSec, flags)
		if st.LastFailedISO != emptyValue {
			fmt.Fprintf(&b, "  last_failed=%s", st.LastFailedISO)
		}
		fmt.Fprintf(&b, "\n")
	}
	if !anyUnhealthy {
		fmt.Fprintf(&b, "  (none)\n")
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}
