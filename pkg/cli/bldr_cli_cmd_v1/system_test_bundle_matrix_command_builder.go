package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemTestBundleMatrixCommandBuilder creates a new system_test_bundle_matrix command
func NewSystemTestBundleMatrixCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for test-bundle-matrix")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
