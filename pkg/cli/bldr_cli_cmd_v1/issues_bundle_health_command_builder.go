package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewIssuesBundleHealthCommandBuilder creates a new issues_bundle_health command
func NewIssuesBundleHealthCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("issues-bundle-health")
	builder.WithShort("Cross-check issues.json against test-bundle health.jsonl")
	help := clipkg.DynamicHelpBuilder("Cross-check issues.json against test-bundle health.jsonl")
	help.WithDescriptionLines("When the scheduler is idle or issues.json still lists old SCH-run job failures, compare each")
	help.WithDescriptionLines("entry to lines in .zqk/logs/scheduler/cvs/test-bundles/health.jsonl. For each bundle")
	help.WithDescriptionLines("fingerprint, the last line in the scanned file wins — so a newer pass (possibly under a new")
	help.WithDescriptionLines("job id) shows as green even if issues.json was never cleared.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines(paths.RewriteCanonicalCLIInvocations("Use together with: zqk scheduler test-failures list, zqk scheduler test-failures health."))
	help.AddExample("Compare issues.json to bundle health", "%s scheduler issues-bundle-health")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddIntFlag("health-limit", "", 100000, "Max successfully parsed JSON rows from health.jsonl (full scan from start; error if exceeded)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	cli.RequireStorage(cmd, false)
	cli.RequireSession(cmd, false)
	cli.RequireSchedulerCheck(cmd, false)
	return cmd
}
