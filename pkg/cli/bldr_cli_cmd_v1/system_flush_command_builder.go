package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemFlushCommandBuilder creates a new system_flush command
func NewSystemFlushCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for flush")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
