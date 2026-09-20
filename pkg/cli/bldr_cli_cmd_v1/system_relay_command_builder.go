package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemRelayCommandBuilder creates a new system_relay command
func NewSystemRelayCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for relay")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
