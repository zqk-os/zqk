package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectWstransCommandBuilder creates a new object_wstrans command
func NewObjectWstransCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("wstrans")
	builder.WithShort("Workstream transition operations")
	help := clipkg.DynamicHelpBuilder("Workstream transition operations")
	help.WithDescriptionLines("View and list workstream transitions. Create/update/delete via object create/update/delete.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Available subcommands:")
	help.WithDescriptionLines("  - show: Display a workstream transition by ID or the first transition")
	help.WithDescriptionLines("  - list: List workstream_transition objects with optional filters")
	help.AddExample("Show first workstream transition", "%s object wstrans show")
	help.AddExample("Show by ID", "%s object wstrans show WST-001")
	help.AddExample("List all workstream transitions", "%s object wstrans list")
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
