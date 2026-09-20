package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAppInitCommandBuilder creates a new app_init command
func NewAppInitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for init")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
