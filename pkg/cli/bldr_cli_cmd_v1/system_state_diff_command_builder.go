package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemStateDiffCommandBuilder creates a new system_state_diff command
func NewSystemStateDiffCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for state-diff")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
