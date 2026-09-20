package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemStatusCommandBuilder creates a new system_status command
func NewSystemStatusCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for status")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
