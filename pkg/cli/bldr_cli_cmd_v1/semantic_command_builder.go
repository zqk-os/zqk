package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSemanticCommandBuilder creates a new semantic command
func NewSemanticCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for semantic")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
