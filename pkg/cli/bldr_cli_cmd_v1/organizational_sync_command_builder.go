package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewOrganizationalSyncCommandBuilder creates a new organizational_sync command
func NewOrganizationalSyncCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("sync")
	builder.WithShort("Sync organizational structure from a file into ZQK storage")
	help := clipkg.DynamicHelpBuilder("Sync organizational structure from a file into ZQK storage")
	help.WithDescriptionLines("Sync organizational structure from a YAML or JSON file into ZQK storage.")
	help.WithDescriptionLines("Reads organization, division, department, team, partnership, or organizational_change")
	help.WithDescriptionLines("objects and creates or updates them (domain:organizational). Use to load structure")
	help.WithDescriptionLines("exported from zqk or another source.")
	help.AddExample("Sync from YAML file (create new objects only)", "%s organizational sync --file org-structure.yaml --mode create_only")
	help.AddExample("Sync with upsert (create or update)", "%s organizational sync --file org-structure.yaml --mode upsert")
	help.AddExample("Dry run to validate file", "%s organizational sync --file org-structure.yaml --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("file", "", "", "Input file path (YAML or JSON array of organizational objects)")
	builder.AddStringFlag("input-format", "", "", "Input format: yaml or json (default yaml)")
	builder.AddStringFlag("mode", "", "", "Sync mode: create_only or upsert (default upsert)")
	builder.AddBoolFlag("dry-run", "", false, "Validate only; do not create or update")
	builder.AddBoolFlag("continue-on-error", "", false, "Continue after per-object errors")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
