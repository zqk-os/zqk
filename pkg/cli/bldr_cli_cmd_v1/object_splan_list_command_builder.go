package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewObjectSplanListCommandBuilder creates a new object_splan_list command
func NewObjectSplanListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("list")
	builder.WithShort("List strategic plans")
	help := clipkg.DynamicHelpBuilder("List strategic plans")
	help.WithDescriptionLines("List strategic_plan objects. Supports the same --filter, --sort-by, --format,")
	help.WithDescriptionLines("and --limit flags as object list.")
	help.AddExample("List all strategic plans", "%s object splan list")
	help.AddExample("List with filter", "%s object splan list --filter status=active")
	help.AddExample("List in JSON format", "%s object splan list --format json")
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
