package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectPplanCommandBuilder creates a new object_pplan command
func NewObjectPplanCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for pplan")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
