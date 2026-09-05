package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemSyncCommandBuilder creates a new system_sync command
func NewSystemSyncCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for sync")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
