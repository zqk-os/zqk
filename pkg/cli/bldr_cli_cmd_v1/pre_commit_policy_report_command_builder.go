package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewPreCommitPolicyReportCommandBuilder creates a new pre_commit_policy_report command
func NewPreCommitPolicyReportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("policy-report")
	builder.WithShort("Show last policy check output (logging/architecture violations)")
	help := clipkg.DynamicHelpBuilder("Show last policy check output (logging/architecture violations)")
	help.WithDescriptionLines("Prints the contents of .zqk/pre-commit/policy-output.txt from the last run of")
	help.WithDescriptionLines("scripts/pre-commit-policy.sh. Use this to see exact violations when the policy")
	help.WithDescriptionLines("category fails (e.g. POL-CODE-007 or architecture compliance). If the file is")
	help.WithDescriptionLines("missing, run ./scripts/pre-commit-policy.sh first.")
	help.AddExample("View policy output after policy check failed", "%s pre-commit policy-report")
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
