package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectNeighborsCommandBuilder creates a new object_neighbors command
func NewObjectNeighborsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("neighbors")
	builder.WithShort("Find immediate neighbors of an object (depth=1)")
	help := clipkg.DynamicHelpBuilder("Find immediate neighbors of an object (depth=1)")
	help.WithDescriptionLines("Find immediate neighbors of an object via reference relationships.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Returns objects directly connected via reference fields (depth=1 only).")
	help.WithDescriptionLines("Supports filtering by direction (outgoing, incoming, or both).")
	help.AddExample("Find outgoing neighbors (objects this references)", "%s object neighbors BLI-626 --direction outgoing")
	help.AddExample("Find incoming neighbors (objects that reference this)", "%s object neighbors BLI-626 --direction incoming")
	help.AddExample("Find all neighbors (both directions)", "%s object neighbors BLI-626 --direction both")
	help.AddExample("Default is both directions", "%s object neighbors BLI-626")
	help.AddExample("Output as JSON", "%s object neighbors BLI-626 --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddStringFlag("direction", "", "both", "Direction to traverse: outgoing (objects this references), incoming (objects that reference this), or both (default: both)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
