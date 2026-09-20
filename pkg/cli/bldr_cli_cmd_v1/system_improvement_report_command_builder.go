package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemImprovementReportCommandBuilder creates a new system_improvement_report command
func NewSystemImprovementReportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("improvement-report")
	builder.WithShort("Generate system improvement report with comparative analysis and prioritized work items")
	help := clipkg.DynamicHelpBuilder("Generate system improvement report with comparative analysis and prioritized work items")
	help.WithDescriptionLines("Generates a comprehensive report to drive system improvement lifecycle and focus.")
	help.WithDescriptionLines("Runs on a cadence and can optionally trigger remediation and adaptive scheduler-cadence tuning.")
	help.AddExample("Generate improvement report", "%s system improvement-report")
	help.AddExample("Generate JSON report without remediation triggers", "%s system improvement-report --format json --trigger-remediation=false")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("report-file", "", "", "Write report to this path (default: .zqk/logs/reports/improvement-report-<timestamp>.{txt,json})")
	builder.AddBoolFlag("emit-events", "", false, "Emit coordinator events for report completion (for dashboards)")
	builder.AddBoolFlag("trigger-remediation", "", true, "After writing the report, enqueue scheduler jobs to address work items")
	builder.AddBoolFlag("apply-adaptive-cadence", "", true, "Apply adaptive cadence recommendation to SCH-improvement-report with change cooldown")
	builder.AddBoolFlag("persist-snapshot-audit", "", true, "Persist compact improvement snapshot to audit_event for queryable history")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
