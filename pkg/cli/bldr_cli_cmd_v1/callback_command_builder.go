package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewCallbackCommandBuilder creates a new callback command
func NewCallbackCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for callback")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
