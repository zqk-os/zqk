package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewSystemSyncGlossaryFromSpecsCommandBuilder creates a new system_sync_glossary_from_specs command
func NewSystemSyncGlossaryFromSpecsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("sync-glossary-from-specs")
	builder.WithShort("Propose (and optionally create) glossary terms from specs/lifecycles/configs")
	help := clipkg.DynamicHelpBuilder("Propose (and optionally create) glossary terms from specs/lifecycles/configs")
	help.WithDescriptionLines("Scans .zqk/specs/objects, lifecycles, and configs to detect glossary candidates")
	help.WithDescriptionLines("that are missing as glossary_term objects. Default is dry-run proposal output only.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Use --apply --dry-run=false to create missing glossary_term rows through storage using")
	help.WithDescriptionLines("the glossary instance builder (no direct YAML edits under .zqk/process/).")
	help.AddExample("Report missing glossary terms inferred from specs", "%s system sync-glossary-from-specs --format table")
	help.AddExample("Create missing glossary terms", "%s system sync-glossary-from-specs --apply --dry-run=false --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("specs-dir", "", "", "Directory with object specs (default: .zqk/specs/objects)")
	builder.AddStringFlag("lifecycles-dir", "", "", "Directory with lifecycle specs (default: .zqk/specs/lifecycles)")
	builder.AddStringFlag("configs-dir", "", "", "Directory with config specs (default: .zqk/specs/configs)")
	builder.AddBoolFlag("dry-run", "", true, "Only report missing glossary candidates; do not create objects")
	builder.AddBoolFlag("apply", "", false, "Create missing glossary terms (requires --dry-run=false)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
