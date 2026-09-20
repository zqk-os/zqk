package ops

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewLevelUpCmd creates the level-up command
func NewLevelUpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "level-up [agent_skill_id]",
		Short: "Run an evaluation loop for an agent skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skillID := args[0]

			projectRoot := paths.ResolveProjectRoot(".")

			st, ok := cli.GetObjectStorageForProjectRoot(projectRoot)
			if !ok || st == nil {
				return fmt.Errorf("storage provider not initialized for project root: %s", projectRoot)
			}

			secCtx := pkgctx.GetSecurityContext(cmd.Context())
			if secCtx == nil {
				// Fallback to system context for debugging if missing
				secCtx = pkgctx.NewSecurityContext(pkgctx.SystemAccountID, []string{"system"}, []string{"*"})
			}

			obj, err := st.Read(cmd.Context(), secCtx, skillID)
			if err != nil {
				return fmt.Errorf("failed to fetch agent_skill %s: %w", skillID, err)
			}

			instructions, ok := obj[objects.FieldKeyInstructions].(string)
			if !ok || instructions == "" {
				return fmt.Errorf("agent_skill %s does not contain valid instructions", skillID)
			}

			content := []byte(instructions)
			skillPath := "kernel://agent_skill/" + skillID

			fmt.Fprintf(cmd.OutOrStdout(), "Evaluating skill: %s...\n", skillID)
			fmt.Fprintf(cmd.OutOrStdout(), "Path: %s\n\n", skillPath)

			client := llm.NewClient(cmd.Context(), nil)

			templateID := "PROMPT-1788723878961338000-d2912e6a"
			templateObj, err := st.Read(cmd.Context(), secCtx, templateID)
			if err != nil {
				return fmt.Errorf("failed to fetch evaluator prompt_template %s: %w", templateID, err)
			}

			systemPrompt, ok := templateObj[objects.FieldKeyPromptBody].(string)
			if !ok || systemPrompt == "" {
				return fmt.Errorf("evaluator prompt_template %s is empty or missing prompt_body", templateID)
			}

			prompt := fmt.Sprintf("Evaluate the following agent skill instructions:\n\n%s", string(content))

			resp, err := client.GenerateCompletion(cmd.Context(), prompt, systemPrompt)
			if err != nil {
				return fmt.Errorf("LLM evaluation failed: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Optimization Report (CEF-Style Prompt Evaluation):\n")
			fmt.Fprintf(cmd.OutOrStdout(), "--------------------------------------------------\n")
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", resp)
			fmt.Fprintf(cmd.OutOrStdout(), "\nConvergence session completed successfully.\n")
			return nil
		},
	}
	return cmd
}
