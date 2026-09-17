package system

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/brand"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
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
	out := cli.CommandOutputWriter(cmd, pkgctx.NewSystemContext())
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	exe := brand.ExecutableName()

	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(out, format+"\n", args...)
	}

	write("%s", green(fmt.Sprintf("=== %s Community Edition Onboarding ===", brand.ProductName())))
	write("Welcome to %s! Follow this tutorial to align on our operational philosophy.", brand.ProductName())
	write("In %s, substantive concepts (goals, requirements, policies) are rigid objects, not unstructured markdown.", brand.ProductName())

	write("%s", cyan("Step 1 - Operational Philosophy"))
	write("Read the policy to understand the Object-First approach:")
	write("  %s", yellow(fmt.Sprintf("%s object list policy --filter \"title~=Operational Philosophy\"", exe)))

	write("%s", cyan("Step 2 - Starter kernel graph (org → mission → vision → goal → workstream → plan)"))
	write("Init should already have this spine. Confirm it, then follow the plan — do not hand-mint a lone goal:")
	write("  %s", yellow(fmt.Sprintf("%s object list organization", exe)))
	write("  %s", yellow(fmt.Sprintf("%s object list mission", exe)))
	write("  %s", yellow(fmt.Sprintf("%s object list vision", exe)))
	write("  %s", yellow(fmt.Sprintf("%s object list goal", exe)))
	write("  %s", yellow(fmt.Sprintf("%s object list workstream", exe)))
	write("  %s", yellow(fmt.Sprintf("%s object list priority_plan", exe)))
	write("If those counts are 0, seed the pipeline: scripts/starter_kernel_graph/seed.sh")

	write("%s", cyan("Step 3 - Current Priorities & Self-Discovery"))
	write("Discover what work is planned and self-discover your next task:")
	write("  %s", yellow(fmt.Sprintf("%s workflow whats-next --format json", exe)))
	write("  %s", yellow(fmt.Sprintf("%s object list backlog_item --filter priority_plan_ref=<PLAN_ID>", exe)))

	write("%s", cyan("Step 4 - Agent host sync (workspace ↔ kernel)"))
	write("Detect your IDE/agent, seed default seating, and prime regenerable directives:")
	write("  %s", yellow(fmt.Sprintf("%s system agent-onboard", exe)))
	write("  %s", yellow("Guide: docs/onboarding/COMMUNITY_FIRST_RUN.md"))
	write("Headless / appliance (no IDE):")
	write("  %s", yellow(fmt.Sprintf("%s system agent-onboard --headless", exe)))
	write("  %s", yellow("Guide: docs/onboarding/EDGE_HEADLESS_FIRST_RUN.md"))

	write("%s", cyan("Step 5 - AI Agent Integration (Zero-Config MCP)"))
	write("Connect your AI IDE (like IDE or Claude) instantly to the ZQK knowledge kernel:")
	write("  %s", yellow(fmt.Sprintf("%s mcp serve", exe)))
	write("  %s", yellow("Config snippet (IDE / Claude Desktop): {\"command\": \""+exe+"\", \"args\": [\"mcp\", \"serve\"]}"))

	write("%s", cyan("Step 6 - System Health & Verification"))
	write("Keep the system running smoothly:")
	write("  %s", yellow(fmt.Sprintf("%s system check", exe)))

	write("When in doubt, query objects first. Enjoy building with the Sovereign OS!")
	return nil
}
