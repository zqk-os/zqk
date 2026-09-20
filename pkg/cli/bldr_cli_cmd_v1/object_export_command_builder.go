package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectExportCommandBuilder creates a new object_export command
func NewObjectExportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for export")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
