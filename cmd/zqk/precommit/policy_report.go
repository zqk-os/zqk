package precommit

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/precommit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewPolicyReportCmd creates the pre-commit policy-report command; prints .zqk/pre-commit/policy-output.txt
// so you can see exact policy violations (logging/architecture) when the policy category fails.
func NewPolicyReportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitPolicyReportCommandBuilder()
	cmd.RunE = runPreCommitPolicyReport
	return cmd
}

func runPreCommitPolicyReport(cmd *cobra.Command, _ []string) error {
	projectRoot, _ := cmd.Flags().GetString("project-root")
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found; run from repo or pass --project-root")
	}
	path := precommit.PolicyOutputPath(projectRoot)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return cli.WriteOutput(cmd, []byte("No policy output yet. Run ./scripts/pre-commit-policy.sh to capture policy check results, then run this command again to view violations (e.g. logging or architecture compliance).\n"))
		}
		return errfmt.Newf("read policy output").Wrap(err)
	}
	return cli.WriteOutput(cmd, data)
}
