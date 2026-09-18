package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectBulkCommandBuilder creates a new object_bulk command
func NewObjectBulkCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for bulk")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
