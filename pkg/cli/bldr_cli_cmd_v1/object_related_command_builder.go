package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewObjectRelatedCommandBuilder creates a new object_related command
func NewObjectRelatedCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("related")
	builder.WithShort("Find objects related to an object via reference fields")
	help := clipkg.DynamicHelpBuilder("Find objects related to an object via reference fields")
	help.WithDescriptionLines("Find objects related to a given object via reference fields.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Traverses relationships following reference fields (*_ref, *_refs) up to a specified depth.")
	help.WithDescriptionLines("Supports filtering by relationship type.")
	help.AddExample("Find all related objects (depth 1)", "%s object related BLI-626")
	help.AddExample("Find related objects up to depth 3", "%s object related BLI-626 --depth 3")
	help.AddExample("Find objects related via specific relationship type", "%s object related BLI-626 --relationship priority_plan_ref")
	help.AddExample("Output as JSON", "%s object related BLI-626 --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddIntFlag("depth", "", 1, "Maximum depth to traverse relationships (default: 1)")
	builder.AddStringFlag("relationship", "", "", "Filter by specific relationship type (e.g., priority_plan_ref)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
