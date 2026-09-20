package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectEvomanCommandBuilder creates a new object_evoman command
func NewObjectEvomanCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("evoman")
	builder.WithShort("Evolution management operations")
	help := clipkg.DynamicHelpBuilder("Evolution management operations")
	help.WithDescriptionLines("View and list evolution_management objects. Create/update/delete via object create/update/delete.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Available subcommands:")
	help.WithDescriptionLines("  - show: Display an evolution management by ID or the first")
	help.WithDescriptionLines("  - list: List evolution_management objects with optional filters")
	help.AddExample("Show first evolution management", "%s object evoman show")
	help.AddExample("Show by ID", "%s object evoman show EVOL-001")
	help.AddExample("List all evolution management", "%s object evoman list")
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
