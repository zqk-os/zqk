package system

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/brand"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

func quickstartExecutable() string {
	exe := brand.ExecutableName()
	if exe == "" {
		return "zqk"
	}
	return exe
}

func communityQuickstartGuide(exe string) string {
	envPrefix := strings.ToUpper(exe)
	return fmt.Sprintf(`🚀 ZQK Community 5-Minute Quickstart (pressure-test binary: %s)

This SKU is not studio zqk. Kernel data stays under .zqk/ (not .zcom/).
Do not export %s_PROJECT_ROOT in your shell profile — it silently attaches every
later command to that checkout instead of the directory you are in.

1. From the project directory, initialize (or skip if .zqk/ already exists):
   $ %s system init --project-name <your-project>

2. Seat your IDE agent:
   $ %s system agent-onboard --format json

3. Pair MCP (one binary — there is no zqk-mcp / %s-mcp):
   $ %s mcp install
   $ %s mcp ensure --tcp 127.0.0.1:8443
   Cursor stdio: %s mcp cursor-adapter

4. Confirm the kernel answers:
   $ %s object list
   $ %s workflow whats-next --format json

Docs: docs/onboarding/COMMUNITY_FIRST_RUN.md
`, exe, envPrefix, exe, exe, exe, exe, exe, exe, exe, exe)
}

func studioQuickstartGuide(exe string) string {
	return fmt.Sprintf(`🚀 ZQK 5-Minute Quickstart

Follow these 4 simple steps to get started:

1. Initialize your project kernel:
   $ %s system init --project-name <your-project> --with-onboarding-roadmap

2. Discover priority plan and next tasks:
   $ %s workflow whats-next

3. Pair with your IDE / AI agent mesh (MCP):
   $ %s mcp install
   Cursor stdio: %s mcp cursor-adapter

4. Verify kernel compliance and system health:
   $ %s system check

Docs: docs/onboarding/COMMUNITY_FIRST_RUN.md
`, exe, exe, exe, exe, exe)
}

func quickstartJSONPayload(exe string) map[string]any {
	if zqkenv.IsCommunityEdition {
		return map[string]any{
			objects.FieldKeyTitle: "ZQK Community Quickstart Guide",
			"steps": []string{
				"1. Initialize: " + exe + " system init --project-name <name>",
				"2. Seat agent: " + exe + " system agent-onboard --format json",
				"3. Pair MCP: " + exe + " mcp install  (then " + exe + " mcp cursor-adapter)",
				"4. Discover work: " + exe + " workflow whats-next --format json",
			},
			"starter_policies": []string{
				"POL-CODE-001: Spec-driven architecture",
				"POL-CODE-007: Structured logging framework",
				"POL-AGENT-001: CLI-only process modifications",
			},
			objects.FieldKeyMcpConfig: map[string]any{
				"cursor": map[string]any{
					"mcpServers": map[string]any{
						"zqk": map[string]any{
							objects.FieldKeyCommand: exe,
							"args":                  []string{"mcp", "cursor-adapter"},
						},
					},
				},
			},
			"docs_url": "docs/onboarding/COMMUNITY_FIRST_RUN.md",
		}
	}
	return map[string]any{
		objects.FieldKeyTitle: "ZQK Quickstart Guide",
		"steps": []string{
			"1. Initialize: " + exe + " system init --project-name <name> --with-onboarding-roadmap",
			"2. Discover priority plan: " + exe + " workflow whats-next",
			"3. Pair MCP server: " + exe + " mcp install",
			"4. Verify health: " + exe + " system check",
		},
		"starter_policies": []string{
			"POL-CODE-001: Spec-driven architecture",
			"POL-CODE-007: Structured logging framework",
			"POL-CODE-009: Test-driven development (TDD)",
			"POL-AGENT-001: CLI-only process modifications",
			"POL-AGENT-TPM-001: Hourglass multi-seat delegation",
		},
		objects.FieldKeyMcpConfig: map[string]any{
			"claude_desktop": map[string]any{
				"mcpServers": map[string]any{
					"zqk": map[string]any{
						objects.FieldKeyCommand: exe,
						"args":                  []string{"mcp", "cursor-adapter"},
					},
				},
			},
		},
		"docs_url": "docs/onboarding/COMMUNITY_FIRST_RUN.md",
	}
}

// NewQuickstartCmd returns the 'quickstart' command which provides a zero-friction
// onboarding and bootstrap walkthrough for new projects and contributors.
func NewQuickstartCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Zero-friction project onboarding and quickstart guide",
		"Prints an interactive walkthrough for bootstrapping and developing with ZQK.",
		"",
		"Guides developers and AI agents through project initialization, kernel inspection,",
		"MCP pairing, starter policies, and session workflow discovery in under 2 minutes.",
	).
		AddExample("Run quickstart guide", "%s quickstart").
		AddExample("JSON format output", "%s quickstart --format json").
		AddExample("Same guide under system", "%s system start-here")

	cmd := &cobra.Command{
		Use:     "quickstart",
		Aliases: []string{"quick-start", "start-here"},
		Short:   "Zero-friction project onboarding and quickstart guide",
		RunE: func(cmd *cobra.Command, args []string) error {
			exe := quickstartExecutable()
			format, _ := cmd.Flags().GetString("format")
			if format == "json" {
				return cli.FormatOutputAs(cmd, cli.FormatJSON, quickstartJSONPayload(exe))
			}
			guide := studioQuickstartGuide(exe)
			if zqkenv.IsCommunityEdition {
				guide = communityQuickstartGuide(exe)
			}
			return cli.WriteOutput(cmd, []byte(guide))
		},
	}
	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().String("format", "text", "Output format (text|json)")

	return cmd
}
