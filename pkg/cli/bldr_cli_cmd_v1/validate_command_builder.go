package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewValidateCommandBuilder creates a new validate command
func NewValidateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for validate")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
