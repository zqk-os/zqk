package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemRetentionToleranceCommandBuilder creates a new system_retention_tolerance command
func NewSystemRetentionToleranceCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for retention-tolerance")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
