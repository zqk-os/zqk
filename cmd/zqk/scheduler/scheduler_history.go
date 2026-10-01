package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// jobStats holds execution statistics for a scheduler job
type jobStats struct {
	JobID        string
	TotalRuns    int
	SuccessCount int
	FailCount    int
	LastRunAt    string
	LastOutcome  string
}

// NewHistoryCmd creates the history command
func NewHistoryCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Show execution history summary for scheduler jobs",
		"Show a summary of scheduler job execution history including success/failure counts and last run time.",
		"",
		"This command queries audit events to show:",
		"- Total runs per job",
		"- Success count",
		"- Failure count",
		"- Last run time",
		"- Last run outcome",
		"",
		"Performance tips:",
		"- Use --limit to reduce the number of events queried (default: 5000)",
		"- Use --since and --until to narrow the time range",
		"- Use --job-id to filter by specific job",
	).
		ExcludeCommonFlags()

	historyCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerHistoryCommandBuilder(), &cobra.Command{Use: "history"})
	helpBuilder.ApplyToCommand(historyCmd)
	cli.BindAsyncProgress(historyCmd, runHistory)
	cli.AddCommonFlags(historyCmd)

	// DNA may already define these; only add when builder is still a stub.
	ensureHistoryFlags(historyCmd)

	return historyCmd
}

func ensureHistoryFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	if f.Lookup("job-id") == nil {
		f.String("job-id", "", "Filter by specific job ID (optional)")
	}
	if f.Lookup("limit") == nil {
		f.Int("limit", 5000, "Maximum number of audit events to query (default: 5000, only used with --bypass-cache or time filters)")
	}
	if f.Lookup("since") == nil {
		f.String("since", "", `Only show history since this time. Supports:
  - RFC3339: 2026-01-01T00:00:00Z
  - Relative: 1h, 12h, 1d, 7d, 30d
  - Natural: today, yesterday, this-week, last-week, this-month, last-month, this-year
  Note: Time filters require --bypass-cache (cache doesn't support time ranges)`)
	}
	if f.Lookup("until") == nil {
		f.String("until", "", `Only show history until this time. Supports:
  - RFC3339: 2026-01-01T00:00:00Z
  - Relative: 1h, 12h, 1d, 7d, 30d
  - Natural: today, yesterday, now
  Note: Time filters require --bypass-cache (cache doesn't support time ranges)`)
	}
	if f.Lookup("bypass-cache") == nil {
		f.Bool("bypass-cache", false, "Bypass cache and query audit events directly (slower, but supports time range filters)")
	}
}

func runHistory(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return showJobHistory(ctx, cmd)
}

// showJobHistory shows execution history summary for scheduler jobs
func showJobHistory(ctx *cli.Context, cmd *cobra.Command) error {
	jhc, err := initializeJobHistoryContext(ctx, cmd)
	if err != nil {
		return err
	}

	var statsByJob map[string]*jobStats

	// Use cache by default (fast), unless bypassed or time filters are specified
	if !jhc.BypassCache {
		statsByJob = aggregateJobStatsFromCache(jhc)
		// If cache is empty, fall back to audit events so history still shows (e.g. cache never populated or different process)
		if len(statsByJob) == 0 {
			events, err := queryAuditEvents(jhc)
			if err != nil {
				return err
			}
			statsByJob = aggregateJobStats(jhc, events)
		}
	} else {
		// Query audit events directly
		events, err := queryAuditEvents(jhc)
		if err != nil {
			return err
		}
		statsByJob = aggregateJobStats(jhc, events)
	}

	if len(statsByJob) == 0 {
		return handleEmptyJobHistory(jhc)
	}

	statsList := make([]*jobStats, 0, len(statsByJob))
	for _, stats := range statsByJob {
		statsList = append(statsList, stats)
	}

	sortJobStatsList(statsList)

	return outputJobHistory(jhc, statsList)
}

// outputHistoryTable outputs job history as a table
func outputHistoryTable(statsList []*jobStats, cmd *cobra.Command) error {
	var buf strings.Builder

	// Get column widths using GetColumnWidth (respects --columns flag, spec display_length, or defaults)
	// Note: We don't have a spec for history output, so we just use defaults and --columns flag
	colID := clipkg.GetColumnWidth("job_id", cmd, nil, 12)
	colTotal := clipkg.GetColumnWidth("total_runs", cmd, nil, 10)
	colSuccess := clipkg.GetColumnWidth("success", cmd, nil, 10)
	colFail := clipkg.GetColumnWidth("failed", cmd, nil, 10)
	colLastRun := clipkg.GetColumnWidth("last_run_at", cmd, nil, 30) // Increased from 25 to 30
	colOutcome := clipkg.GetColumnWidth("last_outcome", cmd, nil, 12)

	// Write header
	fmt.Fprintf(&buf, "%-*s %-*s %-*s %-*s %-*s %-*s\n",
		colID, "Job ID",
		colTotal, "Total Runs",
		colSuccess, "Success",
		colFail, "Failed",
		colLastRun, "Last Run At",
		colOutcome, "Last Outcome")

	totalWidth := colID + colTotal + colSuccess + colFail + colLastRun + colOutcome + 10
	buf.WriteString(strings.Repeat("-", totalWidth) + "\n")

	// Write each job's stats
	for _, stats := range statsList {
		// Format last run time
		lastRun := "Never"
		if stats.LastRunAt != emptyValue {
			// Try to parse and format the timestamp
			// Try RFC3339 first (most common format)
			if t, err := time.Parse(time.RFC3339, stats.LastRunAt); err == nil {
				// Format as compact date/time: "2026-01-03 10:01:53" (no timezone)
				lastRun = t.Format("2006-01-02 15:04:05")
			} else if t, err := time.Parse("2006-01-02T15:04:05Z", stats.LastRunAt); err == nil {
				lastRun = t.Format("2006-01-02 15:04:05")
			} else if t, err := time.Parse("2006-01-02T15:04:05", stats.LastRunAt); err == nil {
				lastRun = t.Format("2006-01-02 15:04:05")
			} else {
				// Fallback: use as-is (will be truncated if needed)
				lastRun = stats.LastRunAt
			}
		}

		// Format outcome
		outcome := stats.LastOutcome
		if outcome == emptyValue {
			outcome = "Unknown"
		}

		// Use TruncateString for consistent truncation
		fmt.Fprintf(&buf, "%-*s %-*d %-*d %-*d %-*s %-*s\n",
			colID, clipkg.TruncateString(stats.JobID, colID),
			colTotal, stats.TotalRuns,
			colSuccess, stats.SuccessCount,
			colFail, stats.FailCount,
			colLastRun, clipkg.TruncateString(lastRun, colLastRun),
			colOutcome, clipkg.TruncateString(outcome, colOutcome))
	}

	fmt.Fprintf(&buf, "\nTotal: %d job(s) with execution history\n", len(statsList))
	return cli.WriteOutput(cmd, []byte(buf.String()))
}
