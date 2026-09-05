package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemStateRestoreCommandBuilder creates a new system_state_restore command
func NewSystemStateRestoreCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for state-restore")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
