package graph

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewGraphCmd creates a new graph command group
func NewGraphCmd() *cobra.Command {
	helpBuilder := cli.DynamicHelpBuilder(
		"Graph-based operations and reasoning",
		"Advanced operations leveraging the graph backend for relationship discovery and reasoning.",
		"",
		"The graph backend enables complex queries across object relationships, dependency chains,",
		"and strategic alignment analysis that are difficult with traditional file-based storage.",
	).
		AddExample("Run a Cypher query", "%s graph query \"MATCH (n:BacklogItem) RETURN n LIMIT 5\"").
		AddSection("Capabilities",
			"• raw Cypher queries\n"+
				"• relationship discovery\n"+
				"• impact analysis\n"+
				"• strategic gap detection",
		)

	graphCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewGraphCommandBuilder(), &cobra.Command{
		Use:   "graph",
		Short: "Graph operations and reasoning",
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(graphCmd)

	// Add subcommands
	graphCmd.AddCommand(NewQueryCmd())
	graphCmd.AddCommand(NewDiscoverCmd())

	return graphCmd
}
