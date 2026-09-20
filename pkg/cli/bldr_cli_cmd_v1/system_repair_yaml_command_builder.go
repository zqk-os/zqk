package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemRepairYamlCommandBuilder creates a new system_repair_yaml command
func NewSystemRepairYamlCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for repair-yaml")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
