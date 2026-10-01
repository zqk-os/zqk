package precommit

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/precommit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewIntegrityReportCmd creates the pre-commit integrity-report command; prints .zqk/pre-commit/integrity-output.txt
// so you can see exact integrity violations (Tier 1 issues, hash mismatches, etc.) when the integrity category fails.
func NewIntegrityReportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitIntegrityReportCommandBuilder()
	cmd.RunE = runPreCommitIntegrityReport
	return cmd
}

func runPreCommitIntegrityReport(cmd *cobra.Command, _ []string) error {
	projectRoot, _ := cmd.Flags().GetString("project-root")
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found; run from repo or pass --project-root")
	}
	path := precommit.IntegrityOutputPath(projectRoot)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return cli.WriteOutput(cmd, []byte("No integrity output yet. Run ./scripts/pre-commit-integrity.sh to capture integrity check results, then run this command again to view violations (e.g. Tier 1 issues, hash mismatches, lifecycle violations).\n"))
		}
		return errfmt.Newf("read integrity output").Wrap(err)
	}
	return cli.WriteOutput(cmd, data)
}
