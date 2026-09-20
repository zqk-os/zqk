package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewUtilityCommandBuilder creates a new utility command
func NewUtilityCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for utility")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
