package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewPreCommitIntegrityReportCommandBuilder creates a new pre_commit_integrity_report command
func NewPreCommitIntegrityReportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("integrity-report")
	builder.WithShort("Show last integrity check output (Tier 1 issues, hash mismatches, etc.)")
	help := clipkg.DynamicHelpBuilder("Show last integrity check output (Tier 1 issues, hash mismatches, etc.)")
	help.WithDescriptionLines("Prints the contents of .zqk/pre-commit/integrity-output.txt from the last run of")
	help.WithDescriptionLines("scripts/pre-commit-integrity.sh. Use this to see exact violations when the integrity")
	help.WithDescriptionLines("category fails (e.g. Tier 1 issues, hash mismatches, lifecycle violations). If the file")
	help.WithDescriptionLines("is missing, run ./scripts/pre-commit-integrity.sh first.")
	help.AddExample("View integrity output after integrity check failed", "%s pre-commit integrity-report")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("project-root", "", "", "Project root (default: resolve from cwd)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
