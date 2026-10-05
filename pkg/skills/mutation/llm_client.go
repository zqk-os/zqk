package mutation

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/llm"
)

const (
	// DefaultSystemPromptCode is the default system prompt used when synthesizing Go skill implementations.
	DefaultSystemPromptCode = "You are a specialized code synthesis AI. Your task is to generate Go code based on the provided prompt and optional parent skills. Only output valid Go code without markdown wrappers unless they are requested. The code should be a complete valid skill."

	// DefaultSystemPromptDocs is the default system prompt used when synthesizing skill documentation.
	DefaultSystemPromptDocs = "You are a technical documentation AI. Your task is to generate concise markdown documentation for the provided Go code."
)

// SkillSynthesisClient adapts ZQK's central llm.Client to synthesize skill code and markdown documentation.
// It strips markdown fencing from model outputs to return compilable Go source code.
type SkillSynthesisClient struct {
	client           llm.Client
	SystemPromptCode string
	SystemPromptDocs string
}

// NewSkillSynthesisClient creates an exposed, configurable skill synthesis client.
func NewSkillSynthesisClient(client llm.Client) *SkillSynthesisClient {
	return &SkillSynthesisClient{
		client:           client,
		SystemPromptCode: DefaultSystemPromptCode,
		SystemPromptDocs: DefaultSystemPromptDocs,
	}
}

// NewRealLLMClient preserves backwards compatibility and returns the LLMClient interface.
func NewRealLLMClient(client llm.Client) LLMClient {
	return NewSkillSynthesisClient(client)
}

// GenerateCode mutates or combines existing skill code into new skill code.
func (c *SkillSynthesisClient) GenerateCode(ctx context.Context, prompt string, parentCode ...string) (string, error) {
	systemPrompt := c.SystemPromptCode
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPromptCode
	}

	var userPrompt strings.Builder
	userPrompt.WriteString("Task: ")
	userPrompt.WriteString(prompt)
	userPrompt.WriteString("\n\n")

	if len(parentCode) > 0 {
		userPrompt.WriteString("Parent Skills:\n")
		for i, code := range parentCode {
			userPrompt.WriteString(fmt.Sprintf("--- Parent %d ---\n", i+1))
			userPrompt.WriteString(code)
			userPrompt.WriteString("\n\n")
		}
	}

	result, err := c.client.GenerateCompletion(ctx, userPrompt.String(), systemPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate code via LLM: %w", err)
	}

	// Clean up markdown block if present
	result = strings.TrimPrefix(result, "```go\n")
	result = strings.TrimPrefix(result, "```go")
	result = strings.TrimPrefix(result, "```\n")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "\n```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	return result, nil
}

// GenerateDocs generates documentation for the newly created skill.
func (c *SkillSynthesisClient) GenerateDocs(ctx context.Context, code string) (string, error) {
	systemPrompt := c.SystemPromptDocs
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPromptDocs
	}

	userPrompt := fmt.Sprintf("Code:\n```go\n%s\n```\n\nPlease write clear, concise documentation explaining what this skill does.", code)

	result, err := c.client.GenerateCompletion(ctx, userPrompt, systemPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate docs via LLM: %w", err)
	}

	return strings.TrimSpace(result), nil
}
