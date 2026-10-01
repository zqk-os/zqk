package scheduler

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/cliapp"

	"github.com/spf13/cobra"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	_ "github.com/zqk-os/zqk/pkg/mcp"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// NewSchedulerCmd creates the scheduler command
func NewSchedulerCmd() *cobra.Command {
	schedulerCmd := bldr_cli_cmd_v1.NewSchedulerCommandBuilder()
	cli.RequireSession(schedulerCmd, false)

	schedulerCmd.AddCommand(NewStartCmd())
	schedulerCmd.AddCommand(NewStopCmd())
	schedulerCmd.AddCommand(NewStatusCmd())
	schedulerCmd.AddCommand(NewTriggerCmd())
	schedulerCmd.AddCommand(NewListCmd())
	schedulerCmd.AddCommand(NewHistoryCmd())
	schedulerCmd.AddCommand(NewActivityCmd())
	schedulerCmd.AddCommand(NewStateCmd())
	schedulerCmd.AddCommand(NewHealthCheckCmd())
	schedulerCmd.AddCommand(NewDumpCmd())
	schedulerCmd.AddCommand(NewIssuesCmd())
	schedulerCmd.AddCommand(NewClearIssuesCmd())
	schedulerCmd.AddCommand(NewEventsCmd())
	schedulerCmd.AddCommand(NewConfigCmd())
	// Host OS unit install/start/status (launchd/systemd) — reliable supervisor path.
	// TRACK: docs/architecture/SCHEDULER_HOST_SERVICE_AND_CLUSTER_STATUS.md
	schedulerCmd.AddCommand(NewServiceCmd())

	registerStudioSchedulerCommands(schedulerCmd)

	return schedulerCmd
}

// NewStartCmd creates the start command
func NewStartCmd() *cobra.Command {
	startCmd := bldr_cli_cmd_v1.NewSchedulerStartCommandBuilder()
	cli.RequireSchedulerCheck(startCmd, false) // start command does its own daemon check
	cli.BindAsyncProgress(startCmd, runStart)
	cli.AddCommonFlags(startCmd)
	// Default: background so the scheduler runs consistently in the background; --foreground to attach.
	startCmd.Flags().Bool("background", true, "Run in background and return immediately (default)")
	startCmd.Flags().Bool("foreground", false, "Run in foreground (attach to terminal and stream logs)")
	startCmd.Flags().Bool("minimal", false, "Run in minimal mode (no background maintenance runner)")

	return startCmd
}

// NewStopCmd creates the stop command
func NewStopCmd() *cobra.Command {
	stopCmd := bldr_cli_cmd_v1.NewSchedulerStopCommandBuilder()
	cli.RequireSchedulerCheck(stopCmd, false) // stop does not need daemon status check
	cli.BindAsyncProgress(stopCmd, runStop)
	cli.AddCommonFlags(stopCmd)

	return stopCmd
}

// NewStatusCmd creates the status command
func NewStatusCmd() *cobra.Command {
	statusCmd := bldr_cli_cmd_v1.NewSchedulerStatusCommandBuilder()
	cli.RequireSchedulerCheck(statusCmd, false) // status checks daemon in RunE; avoid blocking PreRunE
	cli.RequireSession(statusCmd, false)        // avoid session lock contention with running daemon
	cli.BindAsyncProgress(statusCmd, runStatus)
	cli.AddCommonFlags(statusCmd)

	return statusCmd
}

// NewTriggerCmd creates the trigger command
func NewTriggerCmd() *cobra.Command {
	triggerCmd := bldr_cli_cmd_v1.NewSchedulerTriggerCommandBuilder()
	triggerCmd.Args = cobra.ExactArgs(1)
	cli.BindAsyncProgress(triggerCmd, runTrigger)
	cli.AddCommonFlags(triggerCmd)

	return triggerCmd
}

// NewListCmd creates the list command
func NewListCmd() *cobra.Command {
	listCmd := bldr_cli_cmd_v1.NewSchedulerListCommandBuilder()
	cli.BindAsyncProgress(listCmd, runList)
	cli.AddCommonFlags(listCmd)

	return listCmd
}

func runStart(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	// Default: run in background when no flags are set. When user explicitly passes --foreground or
	// --background=false, run in foreground. Using Changed() ensures "zqk scheduler start" with no
	// flags always runs in background (PID created by detached child).
	foreground, _ := cmd.Flags().GetBool("foreground") //nolint:errcheck
	background, _ := cmd.Flags().GetBool("background") //nolint:errcheck
	var runInForeground bool
	if !cmd.Flags().Changed("foreground") && !cmd.Flags().Changed("background") {
		runInForeground = false // default: background
	} else {
		runInForeground = foreground || !background
	}
	if runInForeground {
		return startScheduler(ctx, cmd)
	}
	return startSchedulerInBackground(ctx, cmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return stopScheduler(ctx, cmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return showSchedulerStatus(cmd)
}

func runTrigger(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return triggerJob(ctx, cmd, args[0])
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return listJobs(ctx, cmd)
}

// NewIssuesCmd creates the issues command (show current scheduler issues from .zqk/scheduler/issues.json)
func NewIssuesCmd() *cobra.Command {
	issuesCmd := bldr_cli_cmd_v1.NewSchedulerIssuesCommandBuilder()
	cli.BindAsyncProgress(issuesCmd, runIssues)
	cli.AddCommonFlags(issuesCmd)

	return issuesCmd
}

func runIssues(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		if ctx := cli.GetContext(cmd); ctx != nil && ctx.ProjectRoot != emptyValue {
			projectRoot = ctx.ProjectRoot
		}
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root is required (run from project directory or set ZQK_PROJECT_ROOT)")
	}
	payload, err := schedulerpkg.ReadIssues(projectRoot)
	if err != nil {
		return errfmt.Newf("failed to read issues").Wrap(err)
	}
	if payload == nil {
		okStr := "ok"
		payload = &schedulerpkg.IssuesPayload{Status: okStr}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Status: %s\n", payload.Status)
	fmt.Fprintf(&b, "Updated: %s\n", payload.UpdatedAt)
	if len(payload.Issues) > 0 {
		fmt.Fprintf(&b, "Issues (%d):\n", len(payload.Issues))
		for _, i := range payload.Issues {
			fmt.Fprintf(&b, "  - %s [%s] %s\n    at %s\n", i.JobID, i.JobType, i.Error, i.At)
		}
	} else {
		fmt.Fprintf(&b, "Issues: none\n")
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}

// NewClearIssuesCmd creates the clear-issues command
func NewClearIssuesCmd() *cobra.Command {
	clearIssuesCmd := bldr_cli_cmd_v1.NewSchedulerClearIssuesCommandBuilder()
	cli.BindAsyncProgress(clearIssuesCmd, runClearIssues)
	cli.AddCommonFlags(clearIssuesCmd)

	return clearIssuesCmd
}

func runClearIssues(cmd *cobra.Command, args []string) error {
	// Prefer CWD-based project root so "run from repo" always clears this repo's issues file.
	// Context may have a different project_root from config (e.g. another scenario); clear-issues should act on where the user is.
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		if ctx := cli.GetContext(cmd); ctx != nil && ctx.ProjectRoot != emptyValue {
			projectRoot = ctx.ProjectRoot
		}
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root is required (run from project directory or set ZQK_PROJECT_ROOT)")
	}
	writtenPath, err := schedulerpkg.ClearIssues(projectRoot)
	if err != nil {
		return errfmt.Newf("failed to clear issues").Wrap(err)
	}
	var b strings.Builder
	b.WriteString("Scheduler issues cleared successfully\n")
	if writtenPath != emptyValue {
		fmt.Fprintf(&b, "  %s\n", writtenPath)
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}
