package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewCallbackCommandBuilder creates a new callback command
func NewCallbackCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for callback")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
