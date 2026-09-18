package system

import (
	"fmt"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/brand"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

func renderQuickstartGuide() string {
	exe := brand.ExecutableName()
	return fmt.Sprintf(`🚀 ZQK 5-Minute Quickstart

Follow these 4 simple steps to get started:

1. Initialize your project kernel:
   $ %s system init --project-name <your-project>

2. Discover priority plan and next tasks:
   $ %s workflow whats-next

3. Pair with your IDE / AI agent mesh (MCP):
   Add to claude_desktop_config.json:
   {
     "mcpServers": {
       "%s": {
         "%s": "%s",
         "args": ["mcp", "serve"]
       }
     }
   }

4. Verify kernel compliance and system health:
   $ %s system check

Docs & Architecture: docs/INDEX.md
`, exe, exe, exe, objects.FieldKeyCommand, exe, exe)
}

// NewQuickstartCmd returns the 'zqk quickstart' command which provides a zero-friction
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
		AddExample("JSON format output", "%s quickstart --format json")

	cmd := &cobra.Command{
		Use:     "quickstart",
		Aliases: []string{"quick-start", "start-here"},
		Short:   "Zero-friction project onboarding and quickstart guide",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			if format == "json" {
				exe := brand.ExecutableName()
				initStep := fmt.Sprintf("1. Initialize: %s system init --project-name <name>", exe)
				payload := map[string]any{
					objects.FieldKeyTitle: "ZQK Quickstart Guide",
					"steps": []string{
						initStep,
						fmt.Sprintf("2. Discover priority plan: %s workflow whats-next", exe),
						fmt.Sprintf("3. Pair MCP server: %s mcp serve", exe),
						fmt.Sprintf("4. Verify health: %s system check", exe),
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
			return cli.WriteOutput(cmd, []byte(renderQuickstartGuide()))
		},
	}
	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().String("format", "text", "Output format (text|json)")

	return cmd
}
