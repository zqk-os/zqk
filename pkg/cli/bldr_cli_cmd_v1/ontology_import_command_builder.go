package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewOntologyImportCommandBuilder creates a new ontology_import command
func NewOntologyImportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("import")
	builder.WithShort("Import ontology from RDF/OWL or other format")
	help := clipkg.DynamicHelpBuilder("Import ontology from RDF/OWL or other format")
	help.WithDescriptionLines("Import an ontology from a file (RDF/OWL: Turtle, RDF/XML, JSON-LD).")
	help.WithDescriptionLines("Detects format and reports; full translation to zqk domain ontology is via ITEM-764.")
	help.AddExample("Import RDF/OWL file (auto-detect format)", "%s ontology import --file organizational.owl")
	help.AddExample("Import Turtle file", "%s ontology import --file schema.ttl --input-format rdf_owl")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("file", "", "", "Path to ontology file (.owl, .ttl, .rdf, .jsonld)")
	builder.AddStringFlag("input-format", "", "", "Format: rdf_owl or auto (default auto-detect from extension)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
