package test

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/testrunner"
)

// NewStreamCmd creates the `zqk test stream` command.
func NewStreamCmd() *cobra.Command {
	cmd := bldr.NewTestStreamCommandBuilder()
	cmd.Aliases = []string{"run-progress"}
	cmd.RunE = cli.WithProcessor(runTestStream)
	return cmd
}

func runTestStream(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	var flags clipkg.FlagBag
	parallel := flags.Int(cmd, "parallel")
	pkgPattern := flags.String(cmd, "pkg")
	timeout := flags.Duration(cmd, "test-timeout")
	if err := flags.Err(); err != nil {
		return errfmt.Newf("stream").Wrap(err)
	}

	if len(args) > 0 {
		pkgPattern = args[0]
	}

	opts := testrunner.StreamOptions{
		ProjectRoot: proc.ProjectRoot(),
		Pkg:         pkgPattern,
		Parallel:    parallel,
		Timeout:     timeout,
	}

	summary, err := testrunner.StreamTests(cmd.Context(), opts, cmd.OutOrStdout())
	if err != nil {
		return errfmt.Newf("test run exited with status %d (%d failed)", summary.ExitCode, summary.Failed).Wrap(err)
	}

	return nil
}
