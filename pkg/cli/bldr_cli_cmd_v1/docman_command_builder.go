package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewDocmanCommandBuilder creates a new docman command
func NewDocmanCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for docman")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
