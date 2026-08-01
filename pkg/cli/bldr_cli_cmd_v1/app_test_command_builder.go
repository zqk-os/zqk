package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAppTestCommandBuilder creates a new app_test command
func NewAppTestCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for test")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
