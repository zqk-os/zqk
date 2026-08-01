package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemObjectHygieneScanCommandBuilder creates a new system_object_hygiene_scan command
func NewSystemObjectHygieneScanCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for object-hygiene-scan")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
