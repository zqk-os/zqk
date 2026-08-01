package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAutomationCommandBuilder creates a new automation command
func NewAutomationCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for automation")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
