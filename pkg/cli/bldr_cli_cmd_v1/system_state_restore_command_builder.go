package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemStateRestoreCommandBuilder creates a new system_state_restore command
func NewSystemStateRestoreCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for state-restore")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
