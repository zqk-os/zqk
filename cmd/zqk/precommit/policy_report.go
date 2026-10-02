package precommit

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/precommit"
)

// NewPolicyReportCmd creates the pre-commit policy-report command; prints .zqk/pre-commit/policy-output.txt
// so you can see exact policy violations (logging/architecture) when the policy category fails.
func NewPolicyReportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitPolicyReportCommandBuilder()
	cmd.RunE = runPreCommitPolicyReport
	return cmd
}

func runPreCommitPolicyReport(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolvePreCommitProjectRoot(cmd)
	if err != nil {
		return err
	}
	path := precommit.PolicyOutputPath(projectRoot)
	missingMsg := "No policy output yet. Run ./scripts/pre-commit-policy.sh to capture policy check results, then run this command again to view violations (e.g. logging or architecture compliance).\n"
	return outputPreCommitReportFile(cmd, path, missingMsg, "read policy output")
}
