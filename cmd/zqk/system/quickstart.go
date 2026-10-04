package system

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func renderQuickstartGuide(projectRoot string) string {
	exe := brand.ExecutableName()
	isInit := false
	if projectRoot != "" {
		isInit = paths.IsValidProjectRoot(projectRoot)
	}

	docsLine := docsGuideLine(projectRoot)
	if isInit {
		return fmt.Sprintf(`🚀 ZQK Quickstart

Project Status: Initialized (%s)

Get started in 3 commands:

1. Launch Visual Web Studio:
   $ %s ui -w
   ➜ Open http://127.0.0.1:8080 to inspect your roadmap, DAG, and Gantt timeline.

2. Execute Shovel-Ready Work:
   $ %s do
   ➜ Discovers active priorities, claims the next backlog item, and executes.
   ➜ Or prompt your AI agent: "Let's do a paired walkthrough to capture intent, objectify with '%s new <kind>', and run '%s do'."

3. In-Process Code Search & Token Conservation:
   $ %s grep <query> (alias: %s zgrep)
   ➜ Sub-15ms AST and trigram search with strict token budgeting for AI agents.

(Optional: run '%s system agent-onboard' if adding a new AI editor or agent host)
%s
`, projectRoot, exe, exe, exe, exe, exe, exe, exe, docsLine)
	}

	initFlag := ""
	if exe == "zqk" {
		initFlag = " --with-onboarding-roadmap"
	}
	return fmt.Sprintf(`🚀 ZQK Quickstart

No initialized ZQK project detected in this directory.

Get started in 4 commands:

1. Initialize your project & AI agent directives:
   $ %s system init%s
   ➜ Initializes kernel, seeds starter roadmap, and auto-configures MCP & agents.

2. Launch Visual Web Studio:
   $ %s ui -w
   ➜ Open http://127.0.0.1:8080 to inspect your roadmap, DAG, and Gantt timeline.

3. Execute Shovel-Ready Work:
   $ %s do
   ➜ Autonomously claims and begins the next task (or prompt your AI agent).

4. In-Process Code Search & Token Conservation:
   $ %s grep <query> (alias: %s zgrep)
   ➜ Sub-15ms AST and trigram search with strict token budgeting.

%s
`, exe, initFlag, exe, exe, exe, exe, docsLine)
}

func docsGuideLine(projectRoot string) string {
	if projectRoot != "" && fileutil.Exists(filepath.Join(projectRoot, "docs", "INDEX.md")) {
		return "Docs & Guides: docs/INDEX.md"
	}
	if projectRoot != "" && fileutil.Exists(filepath.Join(projectRoot, "ZQK_GETTING_STARTED.md")) {
		return "Getting Started: ZQK_GETTING_STARTED.md (Online docs: https://docs.zqk.dev)"
	}
	return "Docs & Guides: https://docs.zqk.dev"
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
	projectRoot, _ := resolveCommandProjectRoot(cmd)

	format, _ := cmd.Flags().GetString("format")
	if format == "json" {
		exe := brand.ExecutableName()
		isInit := projectRoot != "" && paths.IsValidProjectRoot(projectRoot)
		initStep := fmt.Sprintf("1. Initialize: %s system init", exe)
		if exe == "zqk" {
			initStep += " --with-onboarding-roadmap"
		}
		steps := []string{
			initStep,
			fmt.Sprintf("2. Web Studio: %s ui -w", exe),
			fmt.Sprintf("3. Execute Work: %s do", exe),
			fmt.Sprintf("4. In-Process Code Search: %s grep <query> (alias: %s zgrep)", exe, exe),
		}
		var activeSteps []string
		if isInit {
			activeSteps = []string{
				fmt.Sprintf("1. Web Studio: %s ui -w", exe),
				fmt.Sprintf("2. Execute Work: %s do", exe),
				fmt.Sprintf("3. In-Process Code Search: %s grep <query> (alias: %s zgrep)", exe, exe),
			}
		}
		payload := map[string]any{
			objects.FieldKeyTitle:  "ZQK Quickstart Guide",
			"project_initialized":  isInit,
			"project_root":         projectRoot,
			"web_ui_url":           "http://127.0.0.1:8080",
			"steps":                steps,
			"active_project_steps": activeSteps,
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
