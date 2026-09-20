package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewRollbackCommandBuilder creates a new rollback command
func NewRollbackCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for rollback")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
