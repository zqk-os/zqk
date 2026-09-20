package llm

import (
	"context"
	"strings"
	"testing"
)

func TestStructuredCompletion_nilClient(t *testing.T) {
	t.Parallel()
	_, err := StructuredCompletion(context.Background(), nil, nil, nil, StructuredCompletionOptions{})
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("err = %v, want nil-client error", err)
	}
}

type optionsStub struct {
	MockLLMClient
	got StructuredCompletionOptions
}

func (s *optionsStub) GenerateStructuredCompletionWithOptions(_ context.Context, _ []Message, _ []ToolDefinition, opts StructuredCompletionOptions) (StructuredCompletionResponse, error) {
	s.got = opts
	return StructuredCompletionResponse{Content: "via-options"}, nil
}

func TestStructuredCompletion_prefersWithOptions(t *testing.T) {
	t.Parallel()
	stub := &optionsStub{}
	res, err := StructuredCompletion(context.Background(), stub, nil, nil, StructuredCompletionOptions{
		ToolChoice:         ToolChoiceRequired,
		ToolChoiceFunction: "zqk_write_code",
	})
	if err != nil {
		t.Fatalf("StructuredCompletion: %v", err)
	}
	if res.Content != "via-options" {
		t.Fatalf("content = %q", res.Content)
	}
	if stub.got.ToolChoice != ToolChoiceRequired || stub.got.ToolChoiceFunction != "zqk_write_code" {
		t.Fatalf("opts = %+v", stub.got)
	}
}

func TestStructuredCompletion_plainClientFallback(t *testing.T) {
	t.Parallel()
	plain := &MockLLMClient{
		GenerateStructuredCompletionFunc: func(context.Context, []Message, []ToolDefinition) (StructuredCompletionResponse, error) {
			return StructuredCompletionResponse{Content: "plain"}, nil
		},
	}
	res, err := StructuredCompletion(context.Background(), plain, nil, nil, StructuredCompletionOptions{ToolChoice: ToolChoiceRequired})
	if err != nil {
		t.Fatalf("StructuredCompletion: %v", err)
	}
	if res.Content != "plain" {
		t.Fatalf("content = %q", res.Content)
	}
}
