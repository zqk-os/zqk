package precommit

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/precommit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewStatusCmd creates the pre-commit status command from spec-driven builder; RunE reads flags and prints status.
func NewStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitStatusCommandBuilder()
	cmd.RunE = runPreCommitStatus
	return cmd
}

func runPreCommitStatus(cmd *cobra.Command, _ []string) error {
	projectRoot, _ := cmd.Flags().GetString("project-root")
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found; run from repo or pass --project-root")
	}

	path := precommit.AggregatedPath(projectRoot)
	_, err := fileutil.Stat(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return printStatusMissing(cmd, projectRoot)
		}
		return err
	}

	a, err := precommit.ReadAggregated(projectRoot)
	if err != nil {
		return errfmt.Newf("read results").Wrap(err)
	}
	return printStatus(cmd, path, a)
}

func printStatusMissing(cmd *cobra.Command, projectRoot string) error {
	var b strings.Builder
	fmt.Fprintln(&b, "Pre-commit results: not found")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Required action:")
	fmt.Fprintln(&b, "  1. Run background checks and aggregate once:")
	fmt.Fprintf(&b, "     %s/scripts/pre-commit-lint.sh\n", projectRoot)
	fmt.Fprintln(&b, "     (Or run pre-commit-integrity.sh and pre-commit-policy.sh as needed)")
	fmt.Fprintln(&b, "  2. Or set up scheduler jobs to run these scripts on a schedule.")
	fmt.Fprintln(&b, "  3. The hook will allow commits until the results file exists (with a warning).")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "See: docs/architecture/PRE_COMMIT_BACKGROUND_RESULTS.md")
	return cli.WriteOutput(cmd, []byte(b.String()))
}

func printStatus(cmd *cobra.Command, path string, a *precommit.AggregatedResult) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Results file: %s\n", path)
	fmt.Fprintf(&b, "Updated at:   %s\n", a.UpdatedAt)
	fmt.Fprintf(&b, "Block:        %v\n", a.Block)
	if a.Block {
		summaries := precommit.BlockingCategoriesSummary(a)
		if len(summaries) > 0 {
			fmt.Fprintln(&b, "")
			fmt.Fprintln(&b, "Blocking categories:")
			for _, s := range summaries {
				fmt.Fprintf(&b, "  - %s\n", s)
			}
			fmt.Fprintln(&b, "")
			fmt.Fprintln(&b, "Required action: Fix the issues above, then either:")
			fmt.Fprintln(&b, paths.RewriteCanonicalCLIInvocations("  zqk pre-commit clear     # clear results and allow commits; next script run repopulates"))
			fmt.Fprintln(&b, paths.RewriteCanonicalCLIInvocations("  zqk pre-commit aggregate # after re-running scripts (e.g. scripts/pre-commit-lint.sh)"))
		}
	} else {
		fmt.Fprintln(&b, "")
		fmt.Fprintln(&b, "Commit would be allowed. To refresh results, run:")
		fmt.Fprintln(&b, paths.RewriteCanonicalCLIInvocations("  zqk pre-commit aggregate"))
		fmt.Fprintln(&b, "  (after background jobs have run)")
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}
