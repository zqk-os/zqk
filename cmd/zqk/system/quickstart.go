package system

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

const quickstartTextGuide = `🚀 ZQK 5-Minute Quickstart

Follow these 4 simple steps to get started:

1. Initialize your project kernel:
   $ zqk system init --project-name <your-project> --with-onboarding-roadmap

2. Discover priority plan and next tasks:
   $ zqk workflow whats-next

3. Pair with your IDE / AI agent mesh (MCP):
   Add to claude_desktop_config.json:
   {
     "mcpServers": {
       "zqk": {
         "command": "zqk-mcp"
       }
     }
   }

4. Verify kernel compliance and system health:
   $ zqk system check

Docs & Architecture: https://github.com/lanceman/zqk#readme
`

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
				payload := map[string]any{
					objects.FieldKeyTitle: "ZQK Quickstart Guide",
					"steps": []string{
						"1. Initialize: zqk system init --project-name <name> --with-onboarding-roadmap",
						"2. Discover priority plan: zqk workflow whats-next",
						"3. Pair MCP server: zqk mcp proxy --tcp 0.0.0.0:7777",
						"4. Verify health: zqk system check",
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
									objects.FieldKeyCommand: "zqk-mcp",
								},
							},
						},
					},
					"docs_url": "https://github.com/lanceman/zqk#readme",
				}
				return cli.FormatOutputAs(cmd, cli.FormatJSON, payload)
			}
			return cli.WriteOutput(cmd, []byte(quickstartTextGuide))
		},
	}
	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().String("format", "text", "Output format (text|json)")

	return cmd
}
