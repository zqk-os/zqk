package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemStateCommitCommandBuilder creates a new system_state_commit command
func NewSystemStateCommitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for state-commit")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
