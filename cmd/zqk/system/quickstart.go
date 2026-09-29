package system

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

func renderQuickstartGuide(projectRoot string) string {
	exe := brand.ExecutableName()
	isInit := false
	if projectRoot != "" {
		isInit = paths.IsValidProjectRoot(projectRoot)
	}

	if isInit {
		return fmt.Sprintf(`🚀 ZQK Greenfield Quickstart & Walkthrough

Project Status: Initialized (%s)
The Knowledge Kernel is active and managing your graph of goals, plans, and tasks.

Follow these 4 simple steps to get immediate value:

1. Launch the Visual Web Studio (Timeline & DAG):
   $ %s ui -w
   ➜ Open http://127.0.0.1:8080 to inspect your Workstreams, Milestones, and Gantt timeline.

2. Onboard & Connect your AI Agents (MCP):
   $ %s system agent-onboard
   ➜ Automatically detects Cursor, VS Code, Cline, Windsurf and primes .agents/AGENTS.md.
   To pair with Claude Desktop or standalone MCP clients:
   $ %s mcp install --client claude-desktop

3. Discover What's Next & Execute Tasks:
   $ %s workflow whats-next        # view the active priority plan and shovel-ready tasks
   $ %s agent claim-work           # claim the next backlog item atomically
   $ %s test run                   # run test verification suites

4. Start Background Services & Verify System Health:
   $ %s scheduler start            # start background scheduler daemons
   $ %s system status              # verify system health and active plan

Documentation & Guides: docs/INDEX.md
`, projectRoot, exe, exe, exe, exe, exe, exe, exe, exe)
	}

	initFlag := ""
	if exe == "zqk" {
		initFlag = " --with-onboarding-roadmap"
	}
	return fmt.Sprintf(`🚀 ZQK 5-Minute Quickstart

No initialized ZQK project detected in this directory.

Follow these 4 simple steps to get started:

1. Initialize your project kernel:
   $ %s system init --project-name <your-project>%s

2. Onboard your AI Agents & IDE:
   $ %s system agent-onboard
   ➜ Automatically configures Cursor, VS Code, Cline, or Windsurf.

3. Launch the Visual Web Studio:
   $ %s ui -w
   ➜ Open http://127.0.0.1:8080 to inspect your Gantt timeline and graph.

4. Discover priority plan and next tasks:
   $ %s workflow whats-next

Documentation & Guides: docs/INDEX.md
`, exe, initFlag, exe, exe, exe)
}

// NewQuickstartCmd returns the 'zqk quickstart' command which provides a zero-friction
// onboarding and bootstrap walkthrough for new projects and contributors.
func NewQuickstartCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemQuickstartCommandBuilder()
	cmd.Aliases = []string{"quick-start", "start-here"}
	if cmd.Flags().Lookup("format") == nil {
		cmd.Flags().String("format", "text", "Output format (text|json)")
	}
	cmd.RunE = runQuickstart
	return cmd
}

func runQuickstart(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	projectRoot := ""
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	projectRoot = ProjectRootOrResolve(projectRoot)

	format, _ := cmd.Flags().GetString("format")
	if format == "json" {
		exe := brand.ExecutableName()
		isInit := projectRoot != "" && paths.IsValidProjectRoot(projectRoot)
		initStep := fmt.Sprintf("1. Initialize: %s system init --project-name <name>", exe)
		if exe == "zqk" {
			initStep += " --with-onboarding-roadmap"
		}
		steps := []string{
			initStep,
			fmt.Sprintf("2. Discover priority plan: %s workflow whats-next", exe),
			fmt.Sprintf("3. Pair MCP server: %s mcp serve", exe),
			fmt.Sprintf("4. Verify health: %s system check", exe),
		}
		var activeSteps []string
		if isInit {
			activeSteps = []string{
				fmt.Sprintf("1. Web Studio: %s ui -w", exe),
				fmt.Sprintf("2. Agent Onboard: %s system agent-onboard", exe),
				fmt.Sprintf("3. What's Next: %s workflow whats-next", exe),
				fmt.Sprintf("4. Scheduler: %s scheduler start", exe),
				fmt.Sprintf("5. Verify: %s system check", exe),
			}
		}
		payload := map[string]any{
			objects.FieldKeyTitle:      "ZQK Quickstart Guide",
			"project_initialized":     isInit,
			"project_root":            projectRoot,
			"web_ui_url":              "http://127.0.0.1:8080",
			"steps":                   steps,
			"active_project_steps":    activeSteps,
			"starter_policies": []string{
				"Spec-driven architecture",
				"Structured logging",
				"Test-driven development (TDD)",
				"CLI-only process modifications",
				"Hourglass multi-seat delegation",
			},
			objects.FieldKeyMcpConfig: map[string]any{
				"claude_desktop": map[string]any{
					"mcpServers": map[string]any{
						exe: map[string]any{
							objects.FieldKeyCommand: exe,
							"args":                  []string{"mcp", "serve"},
						},
					},
				},
			},
			"docs_url": "docs/INDEX.md",
		}
		return cli.FormatOutputAs(cmd, cli.FormatJSON, payload)
	}
	return cli.WriteOutput(cmd, []byte(renderQuickstartGuide(projectRoot)))
}
