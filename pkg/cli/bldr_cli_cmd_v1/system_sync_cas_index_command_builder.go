package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemSyncCasIndexCommandBuilder creates a new system_sync_cas_index command
func NewSystemSyncCasIndexCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for sync-cas-index")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
