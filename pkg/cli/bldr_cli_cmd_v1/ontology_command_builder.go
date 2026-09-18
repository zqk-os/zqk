package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewOntologyCommandBuilder creates a new ontology command
func NewOntologyCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for ontology")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
