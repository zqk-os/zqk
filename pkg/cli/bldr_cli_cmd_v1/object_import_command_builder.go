package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectImportCommandBuilder creates a new object_import command
func NewObjectImportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for import")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
