package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemHydrateGraphCommandBuilder creates a new system_hydrate_graph command
func NewSystemHydrateGraphCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("hydrate-graph")
	builder.WithShort("Hydrate Graph DB from a compressed snapshot")
	help := clipkg.DynamicHelpBuilder("Hydrate Graph DB from a compressed snapshot")
	help.WithDescriptionLines("Expands a state snapshot (.csnap) and writes it directly to the Neo4j/MemGraph database.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
