package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectPathCommandBuilder creates a new object_path command
func NewObjectPathCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("path")
	builder.WithShort("Find a path between two objects via reference relationships")
	help := clipkg.DynamicHelpBuilder("Find a path between two objects via reference relationships")
	help.WithDescriptionLines("Find the shortest path between two objects via reference relationships.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Traverses relationships following reference fields (*_ref, *_refs) to find")
	help.WithDescriptionLines("a connection path between the two objects.")
	help.AddExample("Find path from one backlog item to another", "%s object path BLI-626 BLI-641")
	help.AddExample("Output as JSON", "%s object path BLI-626 BLI-641 --format json")
	help.AddExample("Output as YAML", "%s object path BLI-626 BLI-641 --format yaml")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(2))
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
