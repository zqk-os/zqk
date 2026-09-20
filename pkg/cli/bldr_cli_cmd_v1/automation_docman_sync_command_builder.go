package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAutomationDocmanSyncCommandBuilder creates a new automation_docman_sync command
func NewAutomationDocmanSyncCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for docman-sync")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
