package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/testdrive"
)

// NewTestDriveCmd creates the 'zqk test-drive' (and 'zqk system test-drive') guided developer walkthrough command.
func NewTestDriveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "test-drive",
		Aliases: []string{"td", "testdrive"},
		Short:   "Guided interactive developer test-drive of ZQK autonomous swarms",
		Long: `Interactive guided walkthrough demonstrating how ZQK transforms developer intent
into persistent, verifiable Knowledge Kernel graphs (Vision → Goal → Plan → Req → BLI),
runs concurrent swarm AST verification, and issues tamper-proof cryptographic delivery receipts.

Examples:
  # Run interactive developer test drive
  zqk test-drive

  # Test drive with specific prompt intent
  zqk test-drive --intent "Build an authenticated SQLite task API with telemetry"

  # Headless non-interactive run for CI or quick demonstration
  zqk test-drive --non-interactive --scenario cli-tool
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTestDrive(cmd)
		},
	}

	cmd.Flags().String("intent", "", "Developer intent prompt or feature brief to simulate")
	cmd.Flags().String("scenario", testdrive.ScenarioAPIService, "Preset scenario (api-service, cli-tool, stream-pipeline, custom)")
	cmd.Flags().String("speed", testdrive.SpeedNormal, "Pacing speed for demonstration (normal, fast, instant)")
	cmd.Flags().Bool("non-interactive", false, "Run non-interactively with automatic defaults (recommended for CI/testing)")
	cmd.Flags().Bool("dry-run", false, "Simulate test-drive workflow without disk mutations")

	cli.RequireStorage(cmd, false)
	cli.RequireSession(cmd, false)

	return cmd
}

func runTestDrive(cmd *cobra.Command) error {
	intent, _ := cmd.Flags().GetString("intent")
	scenario, _ := cmd.Flags().GetString("scenario")
	speed, _ := cmd.Flags().GetString("speed")
	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	opts := testdrive.TestDriveOptions{
		Intent:         intent,
		Scenario:       scenario,
		Speed:          speed,
		NonInteractive: nonInteractive,
		DryRun:         dryRun,
		Writer:         cmd.OutOrStdout(),
		Reader:         cmd.InOrStdin(),
	}

	engine, err := testdrive.NewEngine(opts)
	if err != nil {
		return err
	}

	_, err = engine.Run(cmd.Context())
	return err
}
