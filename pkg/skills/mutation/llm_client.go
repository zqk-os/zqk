package mutation

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/llm"
)

// realLLMClient is an implementation of the LLMClient interface using the central llm.Client.
type realLLMClient struct {
	client llm.Client
}

// NewRealLLMClient creates a new realLLMClient.
func NewRealLLMClient(client llm.Client) LLMClient {
	return &realLLMClient{
		client: client,
	}
}

// GenerateCode mutates or combines existing skill code into new skill code.
func (c *realLLMClient) GenerateCode(ctx context.Context, prompt string, parentCode ...string) (string, error) {
	systemPrompt := "You are a specialized code synthesis AI. Your task is to generate Go code based on the provided prompt and optional parent skills. Only output valid Go code without markdown wrappers unless they are requested. The code should be a complete valid skill."

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
func (c *realLLMClient) GenerateDocs(ctx context.Context, code string) (string, error) {
	systemPrompt := "You are a technical documentation AI. Your task is to generate concise markdown documentation for the provided Go code."

	userPrompt := fmt.Sprintf("Code:\n```go\n%s\n```\n\nPlease write clear, concise documentation explaining what this skill does.", code)

	result, err := c.client.GenerateCompletion(ctx, userPrompt, systemPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate docs via LLM: %w", err)
	}

	return strings.TrimSpace(result), nil
}
