package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemCliHooksCommandBuilder creates a new system_cli_hooks command
func NewSystemCliHooksCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for cli-hooks")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
