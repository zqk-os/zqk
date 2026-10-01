package test

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testrunner"
)

// NewCheckContaminationCmd creates the `zqk test check-contamination` command.
func NewCheckContaminationCmd() *cobra.Command {
	cmd := bldr.NewTestCheckContaminationCommandBuilder()
	cmd.Aliases = []string{"contamination-check"}
	cmd.RunE = cli.WithProcessor(runCheckContamination)
	return cmd
}

func runCheckContamination(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	var flags clipkg.FlagBag
	plane := flags.String(cmd, "plane")
	cmdFlag := flags.String(cmd, "command")
	if err := flags.Err(); err != nil {
		return errfmt.Newf("check-contamination").Wrap(err)
	}

	command := args
	if len(command) == 0 && cmdFlag != "" {
		command = []string{"sh", "-c", cmdFlag}
	}

	if len(command) == 0 {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("specify command to run (e.g. zqk test check-contamination -- go test ./pkg/storage)"))
	}

	opts := testrunner.ContaminationCheckOptions{
		ProjectRoot: proc.ProjectRoot(),
		PlanePath:   plane,
	}

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	exitCode, diff, runErr := testrunner.RunWithContaminationCheck(cmd.Context(), opts, command, out, errOut)

	if diff.HasContamination() {
		fmt.Fprintf(errOut, "\n❌ TEST CONTAMINATION: %s was modified by the command!\n", plane)
		fmt.Fprintf(errOut, "   The schema plane must remain strictly read-only during test runs.\n")
		fmt.Fprintf(errOut, "   Contaminated files:\n")
		for _, change := range diff.AllChanged() {
			fmt.Fprintf(errOut, "     • %s\n", change)
		}
		fmt.Fprintf(errOut, "\n   Restore with: git checkout -- %s\n", plane)
		return errfmt.Errorf("test contamination detected: %d file(s) modified in schema plane", len(diff.AllChanged()))
	}

	if runErr != nil {
		return errfmt.Newf("command failed with exit code %d", exitCode).Wrap(runErr)
	}

	fmt.Fprintf(out, "✅ Zero test contamination: %s remained untouched.\n", plane)
	return nil
}
