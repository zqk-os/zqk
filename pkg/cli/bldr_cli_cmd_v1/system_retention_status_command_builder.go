package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemRetentionStatusCommandBuilder creates a new system_retention_status command
func NewSystemRetentionStatusCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for retention-status")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
