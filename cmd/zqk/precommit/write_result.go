package precommit

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/precommit"
	"github.com/zqk-os/zqk/pkg/zqktime"
	"github.com/spf13/cobra"
)

// NewWriteResultCmd creates the pre-commit write-result command from spec-driven builder; RunE reads flags and writes category result.
func NewWriteResultCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitWriteResultCommandBuilder()
	cmd.RunE = runPreCommitWriteResult
	return cmd
}

func runPreCommitWriteResult(cmd *cobra.Command, _ []string) error {
	projectRoot, _ := cmd.Flags().GetString("project-root")
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found; run from repo or pass --project-root")
	}
	category, _ := cmd.Flags().GetString("category")
	if category == emptyValue {
		return errfmt.Errorf("--category is required (e.g. lint, integrity, policy, docman)")
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
