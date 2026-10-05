package ops

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

const skillEvaluatorFallbackPrompt = "You are an evaluator of agent skill instructions. Score clarity, completeness, and kernel-policy alignment. Return concrete rewrite suggestions."

// NewLevelUpCmd creates the level-up command
func NewLevelUpCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewOpsLevelUpCommandBuilder()
	cmd.RunE = runLevelUp
	return cmd
}

func runLevelUp(cmd *cobra.Command, args []string) error {
	skillID := args[0]
	projectRoot := paths.ResolveProjectRoot(".")

	st, ok := cli.GetObjectStorageForProjectRoot(projectRoot)
	if !ok || st == nil {
		return fmt.Errorf("storage provider not initialized for project root: %s", projectRoot)
	}

	secCtx := pkgctx.GetSecurityContext(cmd.Context())
	if secCtx == nil {
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

	fmt.Fprintf(cmd.OutOrStdout(), "Evaluating skill: %s...\n", skillID)
	fmt.Fprintf(cmd.OutOrStdout(), "Path: kernel://agent_skill/%s\n\n", skillID)

	client := llm.NewClient(cmd.Context(), nil)
	systemPrompt := loadSkillEvaluatorPrompt(cmd.Context(), st, secCtx)
	prompt := fmt.Sprintf("Evaluate the following agent skill instructions:\n\n%s", instructions)

	resp, err := client.GenerateCompletion(cmd.Context(), prompt, systemPrompt)
	if err != nil {
		return fmt.Errorf("LLM evaluation failed: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Optimization Report (skill evaluation):\n")
	fmt.Fprintf(cmd.OutOrStdout(), "--------------------------------------------------\n")
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", resp)
	fmt.Fprintf(cmd.OutOrStdout(), "\nConvergence session completed successfully.\n")
	return nil
}

func loadSkillEvaluatorPrompt(ctx context.Context, st storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) string {
	if st == nil {
		return skillEvaluatorFallbackPrompt
	}
	listed, err := st.List(ctx, secCtx, nil, storage.ListFilter{Kind: objects.KindPromptTemplate})
	if err != nil || listed == nil {
		return skillEvaluatorFallbackPrompt
	}
	for _, tmpl := range listed.Objects {
		title, _ := tmpl[objects.FieldKeyTitle].(string)
		lt := strings.ToLower(title)
		if !strings.Contains(lt, "evaluat") || !strings.Contains(lt, "skill") {
			continue
		}
		body, _ := tmpl[objects.FieldKeyPromptBody].(string)
		if strings.TrimSpace(body) != "" {
			return strings.TrimSpace(body)
		}
	}
	return skillEvaluatorFallbackPrompt
}
