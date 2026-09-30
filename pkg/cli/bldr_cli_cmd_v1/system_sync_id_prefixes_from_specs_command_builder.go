package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemSyncIdPrefixesFromSpecsCommandBuilder creates a new system_sync_id_prefixes_from_specs command
func NewSystemSyncIdPrefixesFromSpecsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("sync-id-prefixes-from-specs")
	builder.WithShort("Sync and reconcile ID prefixes in configuration with object specifications")
	help := clipkg.DynamicHelpBuilder("Sync and reconcile ID prefixes in configuration with object specifications")
	help.WithDescriptionLines(
		"Synchronize and reconcile object ID prefixes in kernel configuration",
		"against definitions parsed from schema and object specifications.",
	)
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("apply", "", false, "Apply changes")
	builder.AddBoolFlag("dry-run", "", false, "Dry run mode")
	builder.AddStringFlag("specs-dir", "", "", "Directory with object specs")
	builder.AddStringFlag("config-file", "", "", "Path to config file")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
