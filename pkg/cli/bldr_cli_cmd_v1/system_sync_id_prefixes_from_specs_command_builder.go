package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemSyncIdPrefixesFromSpecsCommandBuilder creates a new system_sync_id_prefixes_from_specs command
func NewSystemSyncIdPrefixesFromSpecsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("sync-id-prefixes-from-specs")
	builder.WithShort("sync-id-prefixes-from-specs command")
	help := clipkg.DynamicHelpBuilder("sync-id-prefixes-from-specs command")
	help.WithDescriptionLines("sync-id-prefixes-from-specs command")
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("apply", "", false, "Apply changes")
	builder.AddBoolFlag("dry-run", "", false, "Dry run mode")
	builder.AddStringFlag("specs-dir", "", "", "Directory with object specs")
	builder.AddStringFlag("config-file", "", "", "Path to config file")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
