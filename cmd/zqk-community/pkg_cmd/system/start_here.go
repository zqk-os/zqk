package system

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/brand"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

// NewStartHereCmd creates the start-here command for onboarding.
func NewStartHereCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewStartHereCommandBuilder()
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)
	cli.BindAsyncProgress(cmd, runStartHere)
	return cmd
}

func runStartHere(cmd *cobra.Command, _ []string) error {
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileHuman))
	exe := brand.ExecutableName()

	logging.Fluent(logger).Info(green(fmt.Sprintf("=== %s Community Edition Onboarding ===", brand.ProductName()))).Log()
	logging.Fluent(logger).Info(fmt.Sprintf("Welcome to %s! Follow this tutorial to align on our operational philosophy.", brand.ProductName())).Log()
	logging.Fluent(logger).Info(fmt.Sprintf("In %s, substantive concepts (goals, requirements, policies) are rigid objects, not unstructured markdown.", brand.ProductName())).Log()

	logging.Fluent(logger).Info(cyan("Step 1 - Operational Philosophy")).Log()
	logging.Fluent(logger).Info("Read the policy to understand the Object-First approach:").Log()
	logging.Fluent(logger).Info("  " + yellow(fmt.Sprintf("%s object list policy --filter \"title~=Operational Philosophy\"", exe))).Log()

	logging.Fluent(logger).Info(cyan("Step 2 - Mission, Vision, and Goals")).Log()
	logging.Fluent(logger).Info("Understand the 'why' by looking at the active goals:").Log()
	logging.Fluent(logger).Info("  " + yellow(fmt.Sprintf("%s object list goal", exe))).Log()
	logging.Fluent(logger).Info("  " + yellow(fmt.Sprintf("%s object create goal --field title=\"Build something awesome\"", exe))).Log()

	logging.Fluent(logger).Info(cyan("Step 3 - Current Priorities & Self-Discovery")).Log()
	logging.Fluent(logger).Info("Discover what work is planned and self-discover your next task:").Log()
	logging.Fluent(logger).Info("  " + yellow(fmt.Sprintf("%s workflow whats-next", exe))).Log()
	logging.Fluent(logger).Info("  " + yellow(fmt.Sprintf("%s object list backlog_item --filter priority_plan_ref=<PLAN_ID>", exe))).Log()

	logging.Fluent(logger).Info(cyan("Step 4 - AI Agent Integration (Zero-Config MCP)")).Log()
	logging.Fluent(logger).Info("Connect your AI IDE (like Cursor or Claude) instantly to the ZQK knowledge kernel:").Log()
	logging.Fluent(logger).Info("  " + yellow(fmt.Sprintf("%s mcp serve", exe))).Log()
	logging.Fluent(logger).Info("  " + yellow("Config snippet (Cursor / Claude Desktop): {\"command\": \""+exe+"\", \"args\": [\"mcp\", \"serve\"]}")).Log()

	logging.Fluent(logger).Info(cyan("Step 5 - System Health & Verification")).Log()
	logging.Fluent(logger).Info("Keep the system running smoothly:").Log()
	logging.Fluent(logger).Info("  " + yellow(fmt.Sprintf("%s system check", exe))).Log()

	logging.Fluent(logger).Info("When in doubt, query objects first. Enjoy building with the Sovereign OS!").Log()
	return nil
}
