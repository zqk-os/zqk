package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewHistoryCommandBuilder creates a new history command
func NewHistoryCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("history")
	builder.WithShort("Show execution history summary for scheduler jobs")
	help := clipkg.DynamicHelpBuilder("Show execution history summary for scheduler jobs")
	help.WithDescriptionLines("Show a summary of scheduler job execution history including success/failure counts and last run time.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This command queries audit events to show:")
	help.WithDescriptionLines("  - Total runs per job")
	help.WithDescriptionLines("  - Success count")
	help.WithDescriptionLines("  - Failure count")
	help.WithDescriptionLines("  - Last run time")
	help.WithDescriptionLines("  - Last run outcome")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Performance tips:")
	help.WithDescriptionLines("  - Use --limit to reduce the number of events queried (default: 5000)")
	help.WithDescriptionLines("  - Use --since and --until to narrow the time range")
	help.WithDescriptionLines("  - Use --job-id to filter by specific job")
	help.AddExample("Show history for all jobs", "%s scheduler history")
	help.AddExample("Show history for a specific job", "%s scheduler history --job-id SCH-001")
	help.AddExample("Show history since last week", "%s scheduler history --since 7d --bypass-cache")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("job-id", "", "", "Filter by specific job ID (optional)")
	builder.AddIntFlag("limit", "", 5000, "Maximum number of audit events to query (default: 5000, only used with --bypass-cache or time filters)")
	builder.AddStringFlag("since", "", "", "Only show history since this time. Supports: RFC3339, Relative (1h, 12h, 1d, 7d, 30d), Natural (today, yesterday, this-week, last-week, this-month, last-month, this-year). Note: Time filters require --bypass-cache")
	builder.AddStringFlag("until", "", "", "Only show history until this time. Supports: RFC3339, Relative (1h, 12h, 1d, 7d, 30d), Natural (today, yesterday, now). Note: Time filters require --bypass-cache")
	builder.AddBoolFlag("bypass-cache", "", false, "Bypass cache and query audit events directly (slower, but more complete for small datasets)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
