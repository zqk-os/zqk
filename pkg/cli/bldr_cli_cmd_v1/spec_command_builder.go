package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSpecCommandBuilder creates a new spec command
func NewSpecCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for spec")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
