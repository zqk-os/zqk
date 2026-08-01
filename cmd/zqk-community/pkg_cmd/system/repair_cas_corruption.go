package system

import (
	"fmt"
	"os/exec"

	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/zqkenv"

	cliinternal "github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/spf13/cobra"
)

func NewRepairCASCorruptionCmd() *cobra.Command {
	var (
		inputFile     string
		dryRun        bool
		quarantineDir string
		stopScheduler bool
		restartOnExit bool
	)

	longHelp := fmt.Sprintf(`Repair Tier-1 CAS filename/content hash mismatches ("CAS file corruption detected").

Input format: TSV with columns:
  object_kind <tab> object_id <tab> file_path <tab> message

This command:
  - Writes a correctly named content-hash file alongside the corrupt file
  - Updates the CAS index mapping (ID -> content hash) under cross-process lock
  - Moves the corrupt file into a quarantine folder for forensic inspection

If the file is already correctly addressed (64-hex filename stem equals SHA-256 of file content), repair is a no-op
(no quarantine; avoids deleting the only blob when re-running repair after content was fixed).

Examples:
  # Dry run from the latest system-health report
  %s system repair-cas-corruption --input-file .zqk/system-health/tier1-cas-corruption.tsv --dry-run

  # Apply repairs (recommended to stop scheduler while repairing scheduler_job objects)
  %s system repair-cas-corruption --input-file .zqk/system-health/tier1-cas-corruption.tsv
`, paths.CLICommandName, paths.CLICommandName)

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemRepairCasCorruptionCommandBuilder(), &cobra.Command{
		Use:   "repair-cas-corruption",
		Short: "Repair CAS filename/content hash mismatches (Tier-1 integrity blockers)",
		Long:  longHelp,
	})
	cliinternal.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		_ = args
		processor, err := cliinternal.NewProcessor(cmd)
		if err != nil {
			return errfmt.Newf("failed to initialize processor").Wrap(err)
		}

		logger := processor.Logger()
		projectRoot := processor.ProjectRoot()

		// Dry-run should be fast and non-invasive.
		// In particular: do not stop/restart the scheduler (that can block and also changes system state).
		if dryRun {
			stopScheduler = false
			restartOnExit = false
		}

		return RunRepairCASCorruptionViaPipeline(cmd, projectRoot, inputFile, dryRun, quarantineDir, stopScheduler, restartOnExit, logger)
	})

	cmd.Flags().StringVar(&inputFile, "input-file", "", "Path to TSV input file (defaults to .zqk/system-health/tier1-cas-corruption.tsv)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be repaired without making changes")
	cmd.Flags().StringVar(&quarantineDir, "quarantine-dir", "", "Directory to move corrupt files into (defaults to .zqk/system-health/quarantine)")
	cmd.Flags().BoolVar(&stopScheduler, "stop-scheduler", false, "Stop scheduler daemon before repairing (recommended for scheduler_job corruption)")
	cmd.Flags().BoolVar(&restartOnExit, "restart-scheduler", false, "Restart scheduler daemon after repair (only if --stop-scheduler is true)")

	return cmd
}

func runSchedulerStopBestEffort(projectRoot string) error {
	c := execCmd(projectRoot, "./zqk", "scheduler", "stop")
	_ = c.Run()
	return nil
}

func runSchedulerStartBestEffort(projectRoot string) error {
	c := execCmd(projectRoot, "./zqk", "scheduler", "start")
	_ = c.Run()
	return nil
}

func execCmd(projectRoot string, name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...) //nolint:gosec // internal command invocation, args not user-tainted
	zqkenv.WireExecForIsolatedProject(c, projectRoot)
	return c
}
