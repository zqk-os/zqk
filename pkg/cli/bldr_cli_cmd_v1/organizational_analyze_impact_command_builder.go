package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewOrganizationalAnalyzeImpactCommandBuilder creates a new organizational_analyze_impact command
func NewOrganizationalAnalyzeImpactCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("analyze-impact")
	builder.WithShort("Analyze impact of organizational changes on ZQK objects")
	help := clipkg.DynamicHelpBuilder("Analyze impact of organizational changes on ZQK objects")
	help.WithDescriptionLines("Analyze the impact of an organizational change on ZQK kernel objects.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This command:")
	help.WithDescriptionLines("  - Reads an organizational_change object")
	help.WithDescriptionLines("  - Identifies affected organizational objects (divisions, teams)")
	help.WithDescriptionLines("  - Finds related ZQK objects (workstreams, goals, backlog_items, milestones)")
	help.WithDescriptionLines("  - Creates an impact_analysis object with the results")
	help.AddExample("Analyze impact of a specific organizational change", "%s organizational analyze-impact --change CHANGE-001")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("change", "", "", "Organizational change object ID (required)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
