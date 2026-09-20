package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemFixHashMismatchesCommandBuilder creates a new system_fix_hash_mismatches command
func NewSystemFixHashMismatchesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for fix-hash-mismatches")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
