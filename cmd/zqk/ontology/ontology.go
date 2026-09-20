package ontology

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewOntologyCmd creates the ontology command group
func NewOntologyCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Ontology import and translation",
		"Ontology operations for importing and translating external ontologies.",
		"",
		"The ontology command group provides tools for:",
		"- Importing RDF/OWL (Turtle, RDF/XML, JSON-LD)",
		"- Translation to zqk domain ontology (BLI-764)",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewOntologyCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "ontology",
	})

	helpBuilder.ApplyToCommand(cmd)

	// Add subcommands
	cmd.AddCommand(NewImportCmd())

	return cmd
}
