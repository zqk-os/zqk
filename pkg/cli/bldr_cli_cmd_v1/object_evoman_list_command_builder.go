package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectEvomanListCommandBuilder creates a new object_evoman_list command
func NewObjectEvomanListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("list")
	builder.WithShort("List evolution management objects")
	help := clipkg.DynamicHelpBuilder("List evolution management objects")
	help.WithDescriptionLines("List evolution_management objects. Supports the same --filter, --sort-by, --format,")
	help.WithDescriptionLines("and --limit flags as object list.")
	help.AddExample("List all evolution management", "%s object evoman list")
	help.AddExample("List with filter", "%s object evoman list --filter status=active")
	help.AddExample("List in JSON format", "%s object evoman list --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(0))
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	builder.WithQueryFlags()
	cmd := builder.Build()
	return cmd
}
