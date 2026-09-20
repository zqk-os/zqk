package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemRetentionToleranceCommandBuilder creates a new system_retention_tolerance command
func NewSystemRetentionToleranceCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for retention-tolerance")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
