package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewIntakeCommandBuilder creates a new intake command
func NewIntakeCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for intake")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
