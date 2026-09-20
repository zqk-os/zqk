package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemValidateCommandBuilder creates a new system_validate command
func NewSystemValidateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for validate")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
