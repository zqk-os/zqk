package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewMatrixCommandBuilder creates a new matrix command
func NewMatrixCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Inspect and update quality matrices and test-bundle registries")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
