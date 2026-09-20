package precommit

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewPreCommitCmd creates the pre-commit command group (aggregate, write-result, status).
// Used by background jobs and the git hook; see docs/architecture/PRE_COMMIT_BACKGROUND_RESULTS.md.
// Subcommands use generated builders from .zqk/cli/specs/pre_commit/ with RunE bound in aggregate.go, write_result.go, status.go.
func NewPreCommitCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Pre-commit background results (aggregate and write category results)",
		"Commands for the pre-commit hook that reads a single results file updated by background jobs.",
		"",
		"Background jobs (e.g. linter per package, integrity check, policy checks) write category",
		"files to "+paths.ProjectDataDir+"/pre-commit/<category>.json. The aggregate command merges them into",
		paths.ProjectDataDir+"/pre-commit/results.json (under "+paths.ProjectDataDir+"/pre-commit/) with one 'block' indicator that the hook reads.",
	).
		AddExample("Merge category results into single file", "%s pre-commit aggregate").
		AddExample("Write lint result after running linter", "%s pre-commit write-result --category=lint --ok=false --summary=\"3 issues\"")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewPreCommitCommandBuilder(), &cobra.Command{
		Use: "pre-commit",
	})
	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(NewAggregateCmd())
	cmd.AddCommand(NewWriteResultCmd())
	cmd.AddCommand(NewWriteResultFromLastCmd())
	cmd.AddCommand(NewStatusCmd())
	cmd.AddCommand(NewClearCmd())
	cmd.AddCommand(NewLintReportCmd())
	cmd.AddCommand(NewPolicyReportCmd())
	cmd.AddCommand(NewIntegrityReportCmd())

	return cmd
}
