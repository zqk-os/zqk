package precommit

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/precommit"
)

// NewWriteResultFromLastCmd creates the write-result-from-last command from spec-driven builder.
// Used by pre-commit job callbacks (when trigger origin is pre_commit): reads the staging
// file written by the check script and runs write-result + aggregate.
func NewWriteResultFromLastCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitWriteResultFromLastCommandBuilder()
	cmd.RunE = runWriteResultFromLast
	return cmd
}

func runWriteResultFromLast(cmd *cobra.Command, _ []string) error {
	projectRoot, category, err := resolvePreCommitCategory(cmd, "lint, policy, integrity")
	if err != nil {
		return err
	}
	result, err := precommit.ReadLastResult(projectRoot, category)
	if err != nil {
		return errfmt.Newf("read last result for %s", category).Wrap(err)
	}
	if err := precommit.WriteCategory(projectRoot, category, result); err != nil {
		return errfmt.Newf("write category").Wrap(err)
	}
	if _, err := precommit.Aggregate(projectRoot); err != nil {
		return errfmt.Newf("aggregate").Wrap(err)
	}
	return nil
}
