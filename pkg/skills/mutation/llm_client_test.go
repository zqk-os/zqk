package mutation

import (
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
)

type mockCentralLLM struct {
	llm.Client     // Embed interface
	completionResp string
	completionErr  error
	receivedPrompt string
	receivedSystem string
}

func (m *mockCentralLLM) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	m.receivedPrompt = prompt
	m.receivedSystem = system
	return m.completionResp, m.completionErr
}

func TestRealLLMClient_GenerateCode(t *testing.T) {
	mock := &mockCentralLLM{
		completionResp: "```go\nfunc NewSkill() {}\n```",
	}

	client := NewRealLLMClient(mock)
	code, err := client.GenerateCode(context.Background(), "create a skill", "func Parent1() {}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != "func NewSkill() {}" {
		t.Errorf("unexpected code: %q", code)
	}

	if !strings.Contains(mock.receivedPrompt, "create a skill") {
		t.Errorf("prompt missing task")
	}
	if !strings.Contains(mock.receivedPrompt, "func Parent1() {}") {
		t.Errorf("prompt missing parent code")
	}
}

func TestRealLLMClient_GenerateDocs(t *testing.T) {
	mock := &mockCentralLLM{
		completionResp: "This is a new skill.",
	}

	client := NewRealLLMClient(mock)
	docs, err := client.GenerateDocs(context.Background(), "func NewSkill() {}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if docs != "This is a new skill." {
		t.Errorf("unexpected docs: %q", docs)
	}

	if !strings.Contains(mock.receivedPrompt, "func NewSkill() {}") {
		t.Errorf("prompt missing code")
	}
}
