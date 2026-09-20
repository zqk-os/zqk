package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAppLoginCommandBuilder creates a new app_login command
func NewAppLoginCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for login")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
