package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewUtilityFixHashesCommandBuilder creates a new utility_fix_hashes command
func NewUtilityFixHashesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for fix-hashes")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
