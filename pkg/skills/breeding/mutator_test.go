package breeding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
)

type mockLLMClient struct {
	llm.Client
	response string
	err      error
}

func (m *mockLLMClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return m.response, m.err
}

func TestLLMSkillMutator_WithLLM(t *testing.T) {
	tmpDir := t.TempDir()
	origFile := filepath.Join(tmpDir, "SKILL.md")
	if err := os.WriteFile(origFile, []byte("# Original Skill\nDo task"), 0644); err != nil {
		t.Fatalf("failed to write original skill: %v", err)
	}

	mutator := NewLLMSkillMutator(&mockLLMClient{
		response: "```markdown\n# Optimized Skill\nDo task better\n```",
	})

	ctx := context.Background()
	newPath, err := mutator.MutateSkill(ctx, origFile, []string{"Low speed", "High tokens"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newPath == origFile {
		t.Errorf("expected distinct new path, got %s", newPath)
	}

	content, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("failed to read mutated file: %v", err)
	}

	if !strings.Contains(string(content), "# Optimized Skill") {
		t.Errorf("unexpected content: %s", string(content))
	}
}

func TestLLMSkillMutator_Fallback(t *testing.T) {
	tmpDir := t.TempDir()
	origFile := filepath.Join(tmpDir, "SKILL.md")
	if err := os.WriteFile(origFile, []byte("# Original Skill\nDo task"), 0644); err != nil {
		t.Fatalf("failed to write original skill: %v", err)
	}

	mutator := NewLLMSkillMutator(nil) // nil LLM tests fallback

	ctx := context.Background()
	newPath, err := mutator.MutateSkill(ctx, origFile, []string{"Low speed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("failed to read mutated file: %v", err)
	}

	if !strings.Contains(string(content), "Feedback applied") {
		t.Errorf("expected fallback comments in content: %s", string(content))
	}
}
