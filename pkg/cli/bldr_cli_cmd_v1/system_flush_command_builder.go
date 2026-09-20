package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemFlushCommandBuilder creates a new system_flush command
func NewSystemFlushCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for flush")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
