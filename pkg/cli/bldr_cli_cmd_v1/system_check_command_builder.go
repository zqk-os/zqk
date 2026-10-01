package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemCheckCommandBuilder creates a new system_check command
func NewSystemCheckCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("check")
	builder.WithShort("Run system checks and validation")
	help := clipkg.DynamicHelpBuilder("Run system checks and validation")
	help.WithDescriptionLines("Run comprehensive system checks to validate configuration, specs, and system integrity.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Checks include:")
	help.WithDescriptionLines("  - Spec validation (object specs, lifecycles, traits)")
	help.WithDescriptionLines("  - Configuration validation (id_prefixes, namespaces, kind_mappings)")
	help.WithDescriptionLines("  - Storage integrity checks")
	help.WithDescriptionLines("  - Cache validation")
	help.WithDescriptionLines("  - Dependency validation")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Use --refresh-cache to rebuild the object-id cache and other validation-related caches.")
	help.WithDescriptionLines("--refresh-cache is honored together with --fast (fast mode skips reference checks only).")
	help.AddExample("Run all system checks", "%s system check")
	help.AddExample("Run checks with verbose output", "%s system check --verbose")
	help.AddExample("Refresh cache and run checks", "%s system check --refresh-cache")
	help.AddExample("Run checks and attempt to fix issues", "%s system check --fix")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("refresh-cache", "", false, "Rebuild object-id cache and validation-related caches; honored with --fast (fast skips ref checks only)")
	builder.AddBoolFlag("cache-only", "", false, "Show cached validation results only (do not run validation, trigger background scan)")
	builder.AddBoolFlag("verbose", "", false, "Show detailed check results")
	builder.AddBoolFlag("fix", "", false, "Attempt to automatically fix issues where possible")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
