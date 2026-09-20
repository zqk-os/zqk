package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewActivityCommandBuilder creates a new activity command
func NewActivityCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("activity")
	builder.WithShort("Show recent job activity including started but not completed jobs")
	help := clipkg.DynamicHelpBuilder("Show recent job activity including started but not completed jobs")
	help.WithDescriptionLines("Show recent scheduler job activity including:")
	help.WithDescriptionLines("- Data freshness (oldest/newest loaded event) and busyness")
	help.WithDescriptionLines("- Missed timer triggers with schedule, approximate interval, and severity hints")
	help.WithDescriptionLines("- Jobs started but not completed (with running-duration and likely-health note)")
	help.WithDescriptionLines("- Timer jobs: cron field breakdown and link to cron format docs")
	help.WithDescriptionLines("- One row per job with latest started/completed/failed (relative times)")
	help.WithDescriptionLines("- Flat recent event list (relative times)")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This helps validate cache refreshes, spot stuck runs, and interpret missed slots in context.")
	help.AddExample("Show recent activity", "%s scheduler activity")
	help.AddExample("Filter to one job and show more events", "%s scheduler activity --job-id SCH-001 --limit 50")
	help.AddExample("Re-print activity every 10 seconds until interrupted", "%s scheduler activity --watch 10s")
	help.AddExample("Only events from the last 24 hours (duration) or since an instant", "%s scheduler activity --since 24h")
	help.AddExample("Narrow table layout", "%s scheduler activity --compact")
	help.AddExample("Tune stuck-job labels (shorter run = likely healthy, longer = investigate)", "%s scheduler activity --stuck-recent-after 5m --stuck-stale-after 1h")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddIntFlag("limit", "", 20, "Maximum number of recent events to show")
	builder.AddStringFlag("job-id", "", "", "Filter by specific job ID (optional)")
	builder.AddBoolFlag("bypass-cache", "", false, "Bypass cache and query audit events directly (slower, but more complete for small datasets)")
	builder.AddStringFlag("watch", "", "0", "If greater than zero, re-print activity after this interval until interrupted (e.g. 10s)")
	builder.AddStringFlag("since", "", "", "Only include events on or after this instant: RFC3339 timestamp, or a duration ago (e.g. 24h, 30m)")
	builder.AddStringFlag("stuck-recent-after", "", "2m", "For started-but-not-completed: runs shorter than this are labeled likely healthy in-flight")
	builder.AddStringFlag("stuck-stale-after", "", "30m", "For started-but-not-completed: runs longer than this are labeled investigate")
	builder.AddBoolFlag("compact", "", false, "Use narrower job ID column and table width")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
