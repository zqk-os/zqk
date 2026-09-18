package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectSplanCommandBuilder creates a new object_splan command
func NewObjectSplanCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("splan")
	builder.WithShort("Strategic plan operations")
	help := clipkg.DynamicHelpBuilder("Strategic plan operations")
	help.WithDescriptionLines("View and list strategic plans. Create/update/delete via object create/update/delete.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Available subcommands:")
	help.WithDescriptionLines("  - show: Display a strategic plan by ID or the first plan (e.g. STRAT-PLAN-001)")
	help.WithDescriptionLines("  - list: List strategic_plan objects with optional filters")
	help.AddExample("Show primary strategic plan", "%s object splan show")
	help.AddExample("Show plan by ID", "%s object splan show STRAT-PLAN-001")
	help.AddExample("List all strategic plans", "%s object splan list")
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
