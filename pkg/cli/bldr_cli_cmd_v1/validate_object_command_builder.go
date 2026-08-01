package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewValidateObjectCommandBuilder creates a new validate_object command
func NewValidateObjectCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Executes embedded object machine_hints to verify completion")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
