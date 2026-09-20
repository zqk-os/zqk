package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAppTestCommandBuilder creates a new app_test command
func NewAppTestCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for test")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
