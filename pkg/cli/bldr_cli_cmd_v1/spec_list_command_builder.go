package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSpecListCommandBuilder creates a new spec_list command
func NewSpecListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for list")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
