package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemStatusCommandBuilder creates a new system_status command
func NewSystemStatusCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for status")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
