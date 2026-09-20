package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemMigrateDsiaCommandBuilder creates a new system_migrate_dsia command
func NewSystemMigrateDsiaCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Migrate legacy hash-based files to object ID files with embedded checksums (DSIA Migration)")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
