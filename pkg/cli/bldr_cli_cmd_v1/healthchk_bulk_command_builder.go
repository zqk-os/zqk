package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewHealthchkBulkCommandBuilder creates a new healthchk_bulk command
func NewHealthchkBulkCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for bulk")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
