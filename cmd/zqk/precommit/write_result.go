package precommit

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/precommit"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// NewWriteResultCmd creates the pre-commit write-result command from spec-driven builder; RunE reads flags and writes category result.
func NewWriteResultCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitWriteResultCommandBuilder()
	cmd.RunE = runPreCommitWriteResult
	return cmd
}

func runPreCommitWriteResult(cmd *cobra.Command, _ []string) error {
	projectRoot, category, err := resolvePreCommitCategory(cmd, "e.g. lint, integrity, policy, docman")
	if err != nil {
		return err
	}
	ok, _ := cmd.Flags().GetBool("ok")
	summary, _ := cmd.Flags().GetString("summary")
	blocking, _ := cmd.Flags().GetBool("blocking")
	details, _ := cmd.Flags().GetStringArray("details")
	result := precommit.CategoryResult{
		OK:        ok,
		Summary:   summary,
		Details:   details,
		Blocking:  blocking,
		UpdatedAt: zqktime.NowRFC3339UTC(),
	}
	return precommit.WriteCategory(projectRoot, category, result)
}
