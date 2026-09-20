package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAppLogoutCommandBuilder creates a new app_logout command
func NewAppLogoutCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for logout")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
