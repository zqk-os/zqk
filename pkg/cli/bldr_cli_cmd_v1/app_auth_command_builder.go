package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAppAuthCommandBuilder creates a new app_auth command
func NewAppAuthCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for auth")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
