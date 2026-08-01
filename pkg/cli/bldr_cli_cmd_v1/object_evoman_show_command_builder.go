package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectEvomanShowCommandBuilder creates a new object_evoman_show command
func NewObjectEvomanShowCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("show")
	builder.WithShort("Show an evolution management by ID or the first")
	help := clipkg.DynamicHelpBuilder("Show an evolution management by ID or the first")
	help.WithDescriptionLines("Display an evolution_management object. If id is provided, show that object; otherwise list")
	help.WithDescriptionLines("evolution_management and show the first.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Respects field-level read permissions (ITEM-642). Output format via --format.")
	help.AddExample("Show first evolution management", "%s object evoman show")
	help.AddExample("Show by ID", "%s object evoman show EVOL-001")
	help.AddExample("Show in JSON format", "%s object evoman show --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.MaximumNArgs(1))
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
