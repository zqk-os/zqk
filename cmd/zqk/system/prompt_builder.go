package system

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
)

func NewPromptBuilderCmd() *cobra.Command {
	var budget int
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemPromptBuilderCommandBuilder(), &cobra.Command{
		Use:   "prompt-builder [query]",
		Short: "Builds a topologically sorted context window prompt using the Vectorization Engine",
		Long: paths.RewriteCanonicalCLIInvocations(`Builds a topologically sorted context window prompt by querying the graph for
initial matches and then walking the structural graph up to the root Policies, returning
the optimal token-budgeted prompt context.

Example:
  zqk system prompt-builder "authentication mechanisms" --budget 4000
`),
		RunE: cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}

			if query == "" {
				return fmt.Errorf("a semantic query string must be provided")
			}

			var err error
			_ = err
			ctx := proc.OperationContext()
			sp := proc.Storage()

			var buf strings.Builder
			buf.WriteString(fmt.Sprintf("Searching Vectorization Engine for: '%s'\n", query))
			buf.WriteString(fmt.Sprintf("Token Budget: %d\n", budget))
			buf.WriteString("--------------------------------------------------\n")

			// Use the agentprompt builder for 'onboarding'
			promptData, buildErr := agentprompt.BuildOnboardingPrompt(ctx, sp, budget)
			if buildErr != nil {
				return fmt.Errorf("failed to build vectorized prompt: %w", buildErr)
			}

			buf.WriteString("GENERATED PROMPT CONTEXT (Topologically Sorted):\n")
			buf.WriteString("--------------------------------------------------\n")
			buf.WriteString(promptData)

			// Natively embed retrieval commands for semantic references
			ids := cli.ScanForObjectIDs([]byte(promptData))
			if len(ids) > 0 {
				buf.WriteString("\n\nEMBEDDED RETRIEVAL COMMANDS:\n")
				buf.WriteString("--------------------------------------------------\n")
				for _, id := range ids {
					buf.WriteString(fmt.Sprintf("%s\n", paths.CLIUsage("object", "get", id)))
				}
			}

			buf.WriteString("\n--------------------------------------------------\n")

			return cli.WriteOutput(cmd, []byte(buf.String()))
		}),
	})
	cmd.Flags().IntVar(&budget, "budget", 3000, "Maximum token budget for the generated prompt context")
	return cmd
}
