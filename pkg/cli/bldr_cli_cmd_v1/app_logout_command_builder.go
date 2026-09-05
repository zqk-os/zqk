package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAppLogoutCommandBuilder creates a new app_logout command
func NewAppLogoutCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for logout")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
