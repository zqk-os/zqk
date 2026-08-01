package swarm

import (
	"context"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/multimodal"
)

type mockClient struct {
	responses []llm.StructuredCompletionResponse
	index     int
}

func (m *mockClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	return "", nil
}

func (m *mockClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (m *mockClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (multimodal.AnalysisResult, error) {
	return multimodal.AnalysisResult{}, nil
}

func (m *mockClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "", nil
}

func (m *mockClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return "", nil
}

func (m *mockClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return 0.0, nil
}

func (m *mockClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (multimodal.AnalysisResult, error) {
	return multimodal.AnalysisResult{}, nil
}

func (m *mockClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return "", nil
}

func (m *mockClient) GenerateStructuredCompletion(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (llm.StructuredCompletionResponse, error) {
	if m.index < len(m.responses) {
		resp := m.responses[m.index]
		m.index++
		return resp, nil
	}
	return llm.StructuredCompletionResponse{Content: "final fallback answer"}, nil
}

type mockExecutor struct{}

func (m *mockExecutor) GetTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	return []llm.ToolDefinition{
		{Name: "test_tool", Description: "a test tool"},
	}, nil
}

func (m *mockExecutor) ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error) {
	return "tool result", nil
}

func (m *mockExecutor) Close() error {
	return nil
}

func TestEngine_Run(t *testing.T) {
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "test_tool", Arguments: "{}"},
				},
			},
			{
				Content: "final answer",
			},
		},
	}
	executor := &mockExecutor{}

	engine := NewEngine(client, executor, 0, "test-engine")
	res, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(res, "final answer") {
		t.Fatalf("expected \"final answer\", got %q", res)
	}
}

func TestEngine_CircuitBreaker(t *testing.T) {
	// Setup a mock client that returns a repeating sequence of tool calls
	// We want tool A (test_tool) then tool B (another_tool) repeating 3 times
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "test_tool", Arguments: `{"file":"foo"}`},
				},
			},
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_2", Name: "another_tool", Arguments: `{}`},
				},
			},
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_3", Name: "test_tool", Arguments: `{"file":"foo"}`},
				},
			},
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_4", Name: "another_tool", Arguments: `{}`},
				},
			},
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_5", Name: "test_tool", Arguments: `{"file":"foo"}`},
				},
			},
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_6", Name: "another_tool", Arguments: `{}`},
				},
			},
			{
				Content: "this should not be reached",
			},
		},
	}
	executor := &mockExecutor{}

	engine := NewEngine(client, executor, 0, "test-engine")
	_, err := engine.Run(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected engine run to fail due to repeating pattern circuit-breaker, but got nil error")
	}

	if !strings.Contains(err.Error(), "circuit breaker tripped") {
		t.Fatalf("expected error to mention 'circuit breaker tripped', got: %v", err)
	}
}

func TestFuzzyToolMatching(t *testing.T) {
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "test tool", Arguments: "{}"},
				},
			},
			{
				Content: "final answer",
			},
		},
	}
	executor := &mockExecutor{}

	engine := NewEngine(client, executor, 0, "test-engine")
	res, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(res, "final answer") {
		t.Fatalf("expected \"final answer\", got %q", res)
	}
}

func TestMissingToolFeedback(t *testing.T) {
	// 1. Mock an LLM that calls an entirely non-existent tool.
	// We want to ensure it gets the error string in the next tool call context,
	// and then responds with a final answer.
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_bad", Name: "zqk_make_coffee", Arguments: "{}"},
				},
			},
			{
				Content: "I understand now",
			},
		},
	}
	executor := &mockExecutor{}

	engine := NewEngine(client, executor, 0, "test-engine")
	res, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(res, "I understand now") {
		t.Fatalf("expected \"I understand now\", got %q", res)
	}
}

type loopTrackingExecutor struct {
}

func (l *loopTrackingExecutor) GetTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	p := DefaultToolPrefix()
	return []llm.ToolDefinition{
		{Name: p + "system_status", Description: "get status"},
		{Name: p + "get_current_priority_plan", Description: "get plan"},
		{Name: p + "get_current_backlog_item", Description: "get item"},
		{Name: p + "object_count", Description: "count"},
		{Name: p + "read_code", Description: "read"},
		{Name: p + "write_file", Description: "write"},
	}, nil
}

func (l *loopTrackingExecutor) ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error) {
	return "ok", nil
}

func (l *loopTrackingExecutor) Close() error {
	return nil
}

func TestEngine_ContextGatheringLoopSoftBlock(t *testing.T) {
	p := DefaultToolPrefix()
	// Setup a mock client that calls different status-querying tools repeatedly.
	// Since they are different, they won't trigger the consecutive duplicate check,
	// but they will trigger the context gathering loop check on the 5th and 6th calls.
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "system_status", Arguments: "{}"}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "get_current_priority_plan", Arguments: "{}"}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "get_current_backlog_item", Arguments: "{}"}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c4", Name: p + "object_count", Arguments: "{}"}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c5", Name: p + "system_status", Arguments: "{}"}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c6", Name: p + "get_current_priority_plan", Arguments: "{}"}}},
			{Content: "loop broken", ToolCalls: nil},
		},
	}
	executor := &loopTrackingExecutor{}

	engine := NewEngine(client, executor, 0, "test-engine")
	res, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(res, "loop broken") {
		t.Fatalf("expected final answer 'loop broken', got %q", res)
	}
}

func TestEngine_RepeatingCycleSoftBlock(t *testing.T) {
	p := DefaultToolPrefix()
	// Mock a repeating period-2 cycle: read_code -> write_file -> read_code -> write_file -> read_code.
	// The 5th call completing the cycle should be intercepted and receive guidance.
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "read_code", Arguments: "{}"}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "write_file", Arguments: "{}"}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "read_code", Arguments: "{}"}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c4", Name: p + "write_file", Arguments: "{}"}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c5", Name: p + "read_code", Arguments: "{}"}}},
			{Content: "cycle broken", ToolCalls: nil},
		},
	}
	executor := &loopTrackingExecutor{}

	engine := NewEngine(client, executor, 0, "test-engine")
	res, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(res, "cycle broken") {
		t.Fatalf("expected final answer 'cycle broken', got %q", res)
	}
}

func TestEngine_RepeatingCycleHardAbort(t *testing.T) {
	p := DefaultToolPrefix()
	// Mock a repeating period-2 cycle that ignores guidance and continues.
	// Since the arguments differ, the exact-match circuit breaker won't trip,
	// but the name-only cycle circuit breaker will.
	// calls:
	// c1: read_code {"file":"a"}
	// c2: write_file {"file":"a"}
	// c3: read_code {"file":"b"}
	// c4: write_file {"file":"b"}
	// c5: read_code {"file":"c"} (SoftBlocked=true)
	// c6: write_file {"file":"c"}
	// c7: read_code {"file":"d"} -> should trigger cycle hard abort because index 4 (c5) was SoftBlocked.
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "read_code", Arguments: `{"file":"a"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "write_file", Arguments: `{"file":"a"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "read_code", Arguments: `{"file":"b"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c4", Name: p + "write_file", Arguments: `{"file":"b"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c5", Name: p + "read_code", Arguments: `{"file":"c"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c6", Name: p + "write_file", Arguments: `{"file":"c"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c7", Name: p + "read_code", Arguments: `{"file":"d"}`}}},
		},
	}
	executor := &loopTrackingExecutor{}

	engine := NewEngine(client, executor, 0, "test-engine")
	_, err := engine.Run(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected cycle circuit breaker to trip, but got no error")
	}

	if !strings.Contains(err.Error(), "cycle circuit breaker tripped") {
		t.Fatalf("expected error to mention 'cycle circuit breaker tripped', got: %v", err)
	}
}
