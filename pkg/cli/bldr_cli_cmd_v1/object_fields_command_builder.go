package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectFieldsCommandBuilder creates a new object_fields command
func NewObjectFieldsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for fields")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
