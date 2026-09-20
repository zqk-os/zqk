package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectCommandBuilder creates a new object command
func NewObjectCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for object")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
