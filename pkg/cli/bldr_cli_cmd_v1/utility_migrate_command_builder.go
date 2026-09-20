package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewUtilityMigrateCommandBuilder creates a new utility_migrate command
func NewUtilityMigrateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for migrate")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
