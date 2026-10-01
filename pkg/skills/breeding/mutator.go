package breeding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
)


// LLMSkillMutator implements MutationProvider by prompting an LLM client
// to refactor and optimize underperforming skill instructions or Go code based on feedback.
type LLMSkillMutator struct {
	llmClient llm.Client
}

// NewLLMSkillMutator creates a new LLMSkillMutator.
func NewLLMSkillMutator(client llm.Client) *LLMSkillMutator {
	return &LLMSkillMutator{llmClient: client}
}

// MutateSkill reads the skill file at originalFilePath, generates an improved version
// guided by feedback, writes the mutated skill to a new file, and returns the new file path.
func (m *LLMSkillMutator) MutateSkill(ctx context.Context, originalFilePath string, feedback []string) (string, error) {
	contentBytes, err := os.ReadFile(originalFilePath)
	if err != nil {
		return "", fmt.Errorf("read original skill %s: %w", originalFilePath, err)
	}
	originalContent := string(contentBytes)

	dir := filepath.Dir(originalFilePath)
	ext := filepath.Ext(originalFilePath)
	base := strings.TrimSuffix(filepath.Base(originalFilePath), ext)
	newFilename := fmt.Sprintf("%s_mutated_%d%s", base, time.Now().UnixNano(), ext)
	newPath := filepath.Join(dir, newFilename)

	var mutatedContent string
	if m.llmClient != nil {
		systemPrompt := "You are an agent skill optimization assistant. Improve the provided skill definition or code based on user feedback and low fitness evaluation results. Output only the improved skill without markdown fencing."
		prompt := fmt.Sprintf("Original Skill (%s):\n```\n%s\n```\n\nPerformance Feedback:\n- %s\n\nProduce an updated, optimized version of this skill that addresses all feedback.",
			filepath.Base(originalFilePath),
			originalContent,
			strings.Join(feedback, "\n- "),
		)
		resp, err := m.llmClient.GenerateCompletion(ctx, prompt, systemPrompt)
		if err == nil && strings.TrimSpace(resp) != "" {
			cleaned := strings.TrimSpace(resp)
			cleaned = strings.TrimPrefix(cleaned, "```markdown\n")
			cleaned = strings.TrimPrefix(cleaned, "```go\n")
			cleaned = strings.TrimPrefix(cleaned, "```\n")
			cleaned = strings.TrimSuffix(cleaned, "\n```")
			cleaned = strings.TrimSuffix(cleaned, "```")
			mutatedContent = strings.TrimSpace(cleaned)
		} else {
			logging.GetLoggerFromContext(ctx).LogWarning("llm mutation failed or returned empty; using deterministic fallback", logging.Error(err))
		}
	}

	// Deterministic fallback if LLM is unavailable or fails (e.g. airgapped mode)
	if mutatedContent == "" {
		mutatedContent = fmt.Sprintf("<!-- Mutated from %s on %s -->\n<!-- Feedback applied:\n%s\n-->\n\n%s",
			filepath.Base(originalFilePath),
			time.Now().UTC().Format(time.RFC3339),
			strings.Join(feedback, "\n"),
			originalContent,
		)
	}

	if err := os.WriteFile(newPath, []byte(mutatedContent), paths.FilePerm644); err != nil {
		return "", fmt.Errorf("write mutated skill %s: %w", newPath, err)
	}



	return newPath, nil
}
