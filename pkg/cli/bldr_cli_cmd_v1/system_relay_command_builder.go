package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemRelayCommandBuilder creates a new system_relay command
func NewSystemRelayCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for relay")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
