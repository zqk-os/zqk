package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemEnsureRetentionJobsCommandBuilder creates a new system_ensure_retention_jobs command
func NewSystemEnsureRetentionJobsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("ensure-retention-jobs")
	builder.WithShort("Ensure maintenance bundle scheduler jobs exist (retention, audit, cache_prewarm, autofix, etc.)")
	help := clipkg.DynamicHelpBuilder("Ensure maintenance bundle scheduler jobs exist (retention, audit, cache_prewarm, autofix, etc.)")
	help.WithDescriptionLines("Creates or fixes scheduler jobs so the maintenance bundle runs on schedule: retention_tolerance,")
	help.WithDescriptionLines("audit_event_aggregation, maintenance WAL trigger, hourly object-count-report, hourly improvement-report,")
	help.WithDescriptionLines("cache_prewarm (SCH-007), autofix_batch_cleanup, autofix_run, and autofix_process_pending. Lists jobs via CLI, then creates from")
	help.WithDescriptionLines("scripts/scheduler_jobs/ templates (or updates job_type) if missing or wrong.")
	help.WithDescriptionLines("See docs/observability/RETENTION_TARGET_VISIBILITY_GAP.md and PARTIAL_IMPLEMENTATION_GAPS.md.")
	help.AddExample("Ensure jobs exist or fix job_type", "%s system ensure-retention-jobs")
	help.AddExample("JSON output", "%s system ensure-retention-jobs --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
