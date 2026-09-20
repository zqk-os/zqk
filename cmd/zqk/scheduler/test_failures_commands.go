package scheduler

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// NewTestFailuresCmd creates the test-failures command
func NewTestFailuresCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List and manage failing tests",
		"List and manage failing tests from recent test runs.",
		"",
		"Data sources:",
		"- Callback logs under .zqk/callbacks (when callbacks are configured)",
		"- Scheduler test-bundle shared events: .zqk/logs/scheduler/cvs/test-bundles/events.jsonl",
		"- Rolling health timeline: .zqk/logs/scheduler/cvs/test-bundles/health.jsonl",
		"",
		"Failed bundle event lines include test_failures and suggested_rerun_commands (copy-paste go test",
		"lines). Subcommands list and rerun consider both callbacks and test-bundle events.",
	).
		AddExample("List failing tests (callbacks + test-bundle events)", "%s scheduler test-failures list").
		AddExample("List failures for a specific package", "%s scheduler test-failures list --package ./pkg/storage").
		AddExample("Re-run only failing tests (creates scheduler jobs)", "%s scheduler test-failures rerun").
		AddExample("Summarize test-bundle health from health.jsonl", "%s scheduler test-failures health").
		AddExample("Read criteria verification evidence + optional apply", "%s scheduler test-failures criteria-evidence").
		AddExample("Convergence measure for automation (JSON)", "%s scheduler convergence measure --format json").
		AddExample("Coordinator overseer: rollup + nested CVS tree + arbitrated prompt line", "%s scheduler convergence overseer --coordinator-session-id CVS-001 --format json").
		AddExample("Analyze failure patterns (optional backlog items)", "%s scheduler test-failures analyze").
		ExcludeCommonFlags()

	testFailuresCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerTestFailuresCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "test-failures",
	})

	helpBuilder.ApplyToCommand(testFailuresCmd)

	testFailuresCmd.AddCommand(NewTestFailuresListCmd())
	testFailuresCmd.AddCommand(NewTestFailuresRerunCmd())
	testFailuresCmd.AddCommand(NewTestFailuresAnalyzeCmd())
	testFailuresCmd.AddCommand(NewTestFailuresHealthCmd())
	testFailuresCmd.AddCommand(NewTestFailuresCriteriaEvidenceCmd())

	return testFailuresCmd
}

// NewTestFailuresCriteriaEvidenceCmd reads criteria_verification_evidence from test-bundles/events.jsonl.
func NewTestFailuresCriteriaEvidenceCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Criteria verification evidence from bundle runs",
		"Parses criteria_verification_evidence lines emitted when SCH-run-* bundles complete with bundle JSON criteria_refs.",
		"",
		"Shows the latest event per CRIT-* id in the scanned window. With --apply --dry-run=false, sets criteria status to validated (force lifecycle override).",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerCriteriaEvidenceCommandBuilder(), &cobra.Command{
		Use: "criteria-evidence",
	})
	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().Int("limit", 800, "Max JSONL rows to scan from the end of events.jsonl")
	cmd.Flags().Bool("apply", false, "Update criteria to validated when evidence shows criteria_verification_satisfied=true")
	cmd.Flags().Bool("dry-run", true, "With --apply, print only unless --dry-run=false")
	cli.BindAsyncProgress(cmd, runCriteriaEvidenceFromCmd)
	cli.AddCommonFlags(cmd)
	return cmd
}

// NewTestFailuresListCmd creates the list command for test failures
func NewTestFailuresListCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List failing tests from recent runs",
		"List failing tests from recent test job callbacks.",
		"",
		"Analyzes callback log files to identify which tests failed and groups them",
		"by package for easy identification.",
	).
		ExcludeCommonFlags()

	// Do not reuse NewSchedulerListCommandBuilder — that is the job-list DNA
	// (object/scheduler list collision fix). TRACK: BLI-1785903708509306000-a6d8dc5b
	cmd := &cobra.Command{
		Use: "list",
	}
	cli.BindAsyncProgress(cmd, runTestFailuresList)

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("package", "", "Filter by package path (e.g., ./pkg/storage)")
	cmd.Flags().Int("limit", 50, "Maximum number of failures to show")
	cmd.Flags().String("since", "24h", "Time range to analyze (e.g., 24h, 7d, 1h)")

	cli.AddCommonFlags(cmd)
	return cmd
}

// NewTestFailuresRerunCmd creates the rerun command for failing tests
func NewTestFailuresRerunCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Re-run only failing tests",
		"Re-run only the tests that failed in recent runs.",
		"",
		"This command:",
		"1. Analyzes recent test failures",
		"2. Groups them by package",
		"3. Schedules new test jobs to run only the failing tests",
	).
		AddExample("Re-run all failing tests", "%s scheduler test-failures rerun").
		AddExample("Re-run failing tests for a specific package", "%s scheduler test-failures rerun --package ./pkg/storage").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerRerunCommandBuilder(), &cobra.Command{
		Use: "rerun",
	})
	cli.BindAsyncProgress(cmd, runTestFailuresRerun)

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("package", "", "Filter by package path (e.g., ./pkg/storage)")
	cmd.Flags().String("since", "24h", "Time range to analyze (e.g., 24h, 7d, 1h)")

	cli.AddCommonFlags(cmd)
	return cmd
}

// NewTestFailuresAnalyzeCmd creates the analyze command for test failures
func NewTestFailuresAnalyzeCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Analyze test failures and create backlog items for persistent issues",
		"Analyze test failure patterns and automatically create backlog items for:",
		"- Persistent failures (tests that fail consistently)",
		"- Flaky tests (tests that fail intermittently)",
		"- High-frequency failures (tests that fail often)",
		"",
		"This command reads test logs, identifies patterns, and creates backlog items",
		"to track improvements needed in the system.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerAnalyzeCommandBuilder(), &cobra.Command{
		Use: "analyze",
	})
	cli.BindAsyncProgress(cmd, runTestFailuresAnalyze)

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("since", "7d", "Time range to analyze (e.g., 24h, 7d, 30d)")
	cmd.Flags().Int("min-failures", 3, "Minimum number of failures to create a backlog item")
	cmd.Flags().Float64("failure-rate", 0.5, "Minimum failure rate (0.0-1.0) to consider a test persistently failing")
	cmd.Flags().Bool("create-backlog-items", false, "Actually create backlog items (default: dry-run)")

	cli.AddCommonFlags(cmd)
	return cmd
}

// NewTestFailuresHealthCmd summarizes test-bundle health from .zqk/logs/scheduler/cvs/test-bundles/health.jsonl.
func NewTestFailuresHealthCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Summarize test-bundle run health",
		"Reads the rolling health timeline written when SCH-run-* bundles complete.",
		"",
		"Uses outcomes (pass, test_fail, timeout) and bundle command fingerprints so you can see",
		"whether recent runs are green or need attention. For copy-paste re-runs, see events.jsonl",
		"field suggested_rerun_commands on failed lines.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerHealthCommandBuilder(), &cobra.Command{
		Use: "health",
	})
	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().Int("limit", 500, "Max JSONL lines to read from the end of health.jsonl")
	cli.BindAsyncProgress(cmd, runTestFailuresHealthFromCmd)
	cli.AddCommonFlags(cmd)
	return cmd
}

func runTestFailuresHealthFromCmd(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return runTestFailuresHealth(ctx, cmd)
}

func runTestFailuresList(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return listTestFailures(ctx, cmd)
}

func runTestFailuresRerun(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return rerunTestFailures(ctx, cmd)
}

func runTestFailuresAnalyze(cmd *cobra.Command, args []string) error {
	cliCtx := cli.GetContext(cmd)
	if cliCtx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return analyzeTestFailures(cliCtx, cmd)
}
