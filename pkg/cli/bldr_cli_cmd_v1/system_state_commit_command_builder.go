package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemStateCommitCommandBuilder creates a new system_state_commit command
func NewSystemStateCommitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for state-commit")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
