package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewUtilityFixHashesCommandBuilder creates a new utility_fix_hashes command
func NewUtilityFixHashesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for fix-hashes")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
