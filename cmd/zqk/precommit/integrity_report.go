package precommit

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/precommit"
)

// NewIntegrityReportCmd creates the pre-commit integrity-report command; prints .zqk/pre-commit/integrity-output.txt
// so you can see exact integrity violations (Tier 1 issues, hash mismatches, etc.) when the integrity category fails.
func NewIntegrityReportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitIntegrityReportCommandBuilder()
	cmd.RunE = runPreCommitIntegrityReport
	return cmd
}

func runPreCommitIntegrityReport(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolvePreCommitProjectRoot(cmd)
	if err != nil {
		return err
	}
	path := precommit.IntegrityOutputPath(projectRoot)
	missingMsg := "No integrity output yet. Run ./scripts/pre-commit-integrity.sh to capture integrity check results, then run this command again to view violations (e.g. Tier 1 issues, hash mismatches, lifecycle violations).\n"
	return outputPreCommitReportFile(cmd, path, missingMsg, "read integrity output")
}
