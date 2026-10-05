package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewObjectWstransListCommandBuilder creates a new object_wstrans_list command
func NewObjectWstransListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("list")
	builder.WithShort("List workstream transitions")
	help := clipkg.DynamicHelpBuilder("List workstream transitions")
	help.WithDescriptionLines("List workstream_transition objects. Supports the same --filter, --sort-by, --format,")
	help.WithDescriptionLines("and --limit flags as object list.")
	help.AddExample("List all workstream transitions", "%s object wstrans list")
	help.AddExample("List with filter", "%s object wstrans list --filter status=planned")
	help.AddExample("List in JSON format", "%s object wstrans list --format json")
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
