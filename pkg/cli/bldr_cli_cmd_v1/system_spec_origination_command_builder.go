package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemSpecOriginationCommandBuilder creates a new system_spec_origination command
func NewSystemSpecOriginationCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for spec-origination")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
