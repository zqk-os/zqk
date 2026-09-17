package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestPreserveToolSchemas(t *testing.T) {
	t.Parallel()
	if PreserveToolSchemas != 0 {
		t.Fatal("PreserveToolSchemas must disable compression")
	}
}

func toolArgsJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

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

func (m *mockClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
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

func (m *mockClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
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

func TestEngine_RequiredToolCallRejectsNarrativeOnlyCompletion(t *testing.T) {
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "I implemented and tested the change."},
			{Content: "The task is complete."},
		},
	}

	engine := NewEngine(client, &mockExecutor{}, 0, "test-engine").
		RequireSuccessfulToolCalls(1)
	_, err := engine.Run(context.Background(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "minimum successful tool calls not met") {
		t.Fatalf("expected minimum tool-call error, got %v", err)
	}
	if client.index != 2 {
		t.Fatalf("completion calls = %d, want one corrective retry", client.index)
	}
}

func TestEngine_RequiredToolCallAllowsCompletionAfterExecution(t *testing.T) {
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "I implemented and tested the change."},
			{
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "test_tool", Arguments: "{}"},
				},
			},
			{Content: "Evidence-backed completion."},
		},
	}

	engine := NewEngine(client, &mockExecutor{}, 0, "test-engine").
		RequireSuccessfulToolCalls(1)
	got, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Evidence-backed completion." {
		t.Fatalf("result = %q", got)
	}
}

func TestEngine_RequiredMutationToolRejectsStatusOnlyEvidence(t *testing.T) {
	p := DefaultToolPrefix()
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: p + "system_status", Arguments: "{}"},
				},
			},
			{Content: "Task complete after status check."},
			{Content: "Still done."},
			{Content: "Park after the second write nudge."},
		},
	}
	executor := &namedToolsExecutor{tools: []string{p + "system_status", p + "write_file", p + "write_code"}}

	engine := NewEngine(client, executor, 0, "test-engine").
		RequireSuccessfulToolCalls(1).
		RequireAnySuccessfulTools(MutationEvidenceTools()...)
	_, err := engine.Run(context.Background(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "required mutation tool evidence not met") {
		t.Fatalf("expected mutation evidence error, got %v", err)
	}
}

func TestEngine_RequiredMutationToolSecondNudgeAllowsWrite(t *testing.T) {
	p := DefaultToolPrefix()
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: p + "system_status", Arguments: "{}"},
				},
			},
			{Content: "Task complete after status check."},
			{Content: "Still done after first nudge."},
			{
				ToolCalls: []llm.ToolCall{
					{ID: "call_2", Name: p + "write_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "x.go"})},
				},
			},
			{Content: "Wrote file after second nudge."},
		},
	}
	executor := &namedToolsExecutor{tools: []string{p + "system_status", p + "write_file", p + "write_code"}}

	engine := NewEngine(client, executor, 0, "test-engine").
		RequireSuccessfulToolCalls(1).
		RequireAnySuccessfulTools(MutationEvidenceTools()...)
	got, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Wrote file after second nudge." {
		t.Fatalf("result = %q", got)
	}
}

func TestIsMutationEvidenceTool_acceptsChannelAndProductPrefixes(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"zqk_write_file", "zqk-stable_write_code", "zqk_write_code"} {
		if !IsMutationEvidenceTool(name) {
			t.Fatalf("%q should count as mutation evidence", name)
		}
	}
	if IsMutationEvidenceTool("zqk_system_status") {
		t.Fatal("status probes are not mutation evidence")
	}
}

func TestToolNameAllowed_acceptsProductWriteWhenStableNameRequired(t *testing.T) {
	t.Parallel()
	allow := []string{"zqk-stable_write_code", "zqk-stable_write_file"}
	if !toolNameAllowed("zqk_write_file", allow) {
		t.Fatal("MCP zqk_write_file must satisfy zqk-stable_* evidence")
	}
	if toolNameAllowed("zqk_system_status", allow) {
		t.Fatal("status must not satisfy mutation evidence")
	}
}

func TestEngine_RequiredMutationToolAllowsWriteFileEvidence(t *testing.T) {
	p := DefaultToolPrefix()
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: p + "write_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "x.go"})},
				},
			},
			{Content: "Wrote file; done."},
		},
	}
	executor := &namedToolsExecutor{tools: []string{p + "system_status", p + "write_file", p + "write_code"}}

	engine := NewEngine(client, executor, 0, "test-engine").
		RequireSuccessfulToolCalls(1).
		RequireAnySuccessfulTools(MutationEvidenceTools()...)
	got, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Wrote file; done." {
		t.Fatalf("result = %q", got)
	}
}

func TestEngine_CompletionVerifierReplaysFeedbackThenAccepts(t *testing.T) {
	p := DefaultToolPrefix()
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "write_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go"})}}},
			{Content: "Wrote it; done."},
			{ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "write_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go"})}}},
			{Content: "Fixed the compile error; done."},
		},
	}
	executor := &namedToolsExecutor{tools: []string{p + "write_file"}}

	calls := 0
	engine := NewEngine(client, executor, 0, "test-engine").
		VerifyCompletionWith(func(ctx context.Context, history []ToolCallRecord) (string, error) {
			calls++
			if calls == 1 {
				return "does not compile: undefined: http.DefaultRequest", nil
			}
			return "", nil
		}, seatWorkerRepairBudgetForTest)

	got, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Fixed the compile error; done." {
		t.Fatalf("result = %q, want the post-repair answer", got)
	}
	if calls != 2 {
		t.Fatalf("verifier calls = %d, want 2", calls)
	}
}

func TestEngine_CompletionVerifierFailsAfterRepairBudget(t *testing.T) {
	p := DefaultToolPrefix()
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "write_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go"})}}},
			{Content: "done"},
			{ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "write_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "b.go"})}}},
			{Content: "done again"},
		},
	}
	executor := &namedToolsExecutor{tools: []string{p + "write_file"}}

	engine := NewEngine(client, executor, 0, "test-engine").
		VerifyCompletionWith(func(ctx context.Context, history []ToolCallRecord) (string, error) {
			return "still broken", nil
		}, 1)

	_, err := engine.Run(context.Background(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "completion rejected") {
		t.Fatalf("expected completion rejection, got %v", err)
	}
}

// seatWorkerRepairBudgetForTest mirrors the seat worker's repair allowance.
const seatWorkerRepairBudgetForTest = 2

type namedToolsExecutor struct {
	tools []string
}

func (m *namedToolsExecutor) GetTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	out := make([]llm.ToolDefinition, 0, len(m.tools))
	for _, name := range m.tools {
		out = append(out, llm.ToolDefinition{Name: name, Description: name})
	}
	return out, nil
}

func (m *namedToolsExecutor) ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error) {
	return "tool result", nil
}

func (m *namedToolsExecutor) Close() error {
	return nil
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
	// Distinct arguments: name-only detector needs three periods before soft
	// guidance. After that soft inject, the model stops cycling and answers.
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "read_code", Arguments: `{"file":"a"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "write_file", Arguments: `{"file":"a"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "read_code", Arguments: `{"file":"b"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c4", Name: p + "write_file", Arguments: `{"file":"b"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c5", Name: p + "read_code", Arguments: `{"file":"c"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c6", Name: p + "write_file", Arguments: `{"file":"c"}`}}}, // soft guidance
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
	// Name-only cycles need three periods before soft guidance, then one more
	// period that still matches to hard-abort after SoftBlocked was set.
	client := &mockClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "read_code", Arguments: `{"file":"a"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "write_file", Arguments: `{"file":"a"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "read_code", Arguments: `{"file":"b"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c4", Name: p + "write_file", Arguments: `{"file":"b"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c5", Name: p + "read_code", Arguments: `{"file":"c"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c6", Name: p + "write_file", Arguments: `{"file":"c"}`}}}, // soft trip
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c7", Name: p + "read_code", Arguments: `{"file":"d"}`}}},
			{Content: "", ToolCalls: []llm.ToolCall{{ID: "c8", Name: p + "write_file", Arguments: `{"file":"d"}`}}}, // hard abort
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

// TST-1789075163388619000-6e05e113 / CRIT-REDACTED: Verify 3-Strikes Tool Denial Guard
func TestEngine_VerifyThreeStrikesToolDenialGuard(t *testing.T) {
	p := DefaultToolPrefix()
	tools := []llm.ToolDefinition{
		{Name: p + "read_code", Description: "read code"},
		{Name: p + "execute_bash", Description: "run bash"},
	}

	t.Run("allowlist_deny_abort", func(t *testing.T) {
		client := &mockClient{
			responses: []llm.StructuredCompletionResponse{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "read_code", Arguments: `{"path":"/secret/file"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "read_code", Arguments: `{"path":"/secret/file2"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "read_code", Arguments: `{"path":"/secret/file3"}`}}},
				{Content: "should not be reached"},
			},
		}
		executor := &mockCallExecutor{
			tools: tools,
			fn: func(ctx context.Context, call llm.ToolCall) (string, error) {
				return "ALLOWLIST DENY: path outside sandbox root", nil
			},
		}

		engine := NewEngine(client, executor, 0, "test-watchdog")
		_, err := engine.Run(context.Background(), "sys", "user")
		if err == nil {
			t.Fatal("expected 3 consecutive allowlist denials to abort, got nil")
		}
		if !strings.Contains(err.Error(), "tool_denial_guard: 3 consecutive blocked tool calls") {
			t.Fatalf("expected tool denial abort message, got: %v", err)
		}
	})

	t.Run("soft_block_abort", func(t *testing.T) {
		client := &mockClient{
			responses: []llm.StructuredCompletionResponse{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "execute_bash", Arguments: `{"command":"python script1.py"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "execute_bash", Arguments: `{"command":"python script2.py"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "execute_bash", Arguments: `{"command":"python script3.py"}`}}},
				{Content: "should not be reached"},
			},
		}
		executor := &mockCallExecutor{
			tools: tools,
			fn: func(ctx context.Context, call llm.ToolCall) (string, error) {
				return "ok", nil
			},
		}

		engine := NewEngine(client, executor, 0, "test-watchdog")
		_, err := engine.Run(context.Background(), "sys", "user")
		if err == nil {
			t.Fatal("expected 3 consecutive soft-blocks to abort, got nil")
		}
		if !strings.Contains(err.Error(), "tool_denial_guard: 3 consecutive blocked tool calls") {
			t.Fatalf("expected tool denial abort message, got: %v", err)
		}
	})

	t.Run("interrupted_denials_resets", func(t *testing.T) {
		client := &mockClient{
			responses: []llm.StructuredCompletionResponse{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "read_code", Arguments: `{"path":"/secret/1"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "read_code", Arguments: `{"path":"/secret/2"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "read_code", Arguments: `{"path":"pkg/swarm/engine.go"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c4", Name: p + "read_code", Arguments: `{"path":"/secret/3"}`}}},
				{Content: "success after reset"},
			},
		}
		callCount := 0
		executor := &mockCallExecutor{
			tools: tools,
			fn: func(ctx context.Context, call llm.ToolCall) (string, error) {
				callCount++
				if callCount == 3 {
					return "package swarm", nil
				}
				return "ALLOWLIST DENY: path forbidden", nil
			},
		}

		engine := NewEngine(client, executor, 0, "test-watchdog")
		res, err := engine.Run(context.Background(), "sys", "user")
		if err != nil {
			t.Fatalf("expected success when denials are interrupted, got err: %v", err)
		}
		if res != "success after reset" {
			t.Fatalf("expected 'success after reset', got %q", res)
		}
	})
}

// TST-1789075168791184000-fd8b6784 / CRIT-REDACTED: Verify Hallucination Steering Post-Hook
func TestEngine_VerifyHallucinationSteeringPostHook(t *testing.T) {
	p := DefaultToolPrefix()
	tools := []llm.ToolDefinition{
		{Name: p + "read_code", Description: "read code"},
	}

	t.Run("three_strikes_steering_injection", func(t *testing.T) {
		recClient := &recordingMockClient{
			responses: []llm.StructuredCompletionResponse{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "read_code", Arguments: `{"path":"missing1.go"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "read_code", Arguments: `{"path":"missing2.go"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "read_code", Arguments: `{"path":"missing3.go"}`}}},
				{Content: "recovered after steering"},
			},
		}
		executor := &mockCallExecutor{
			tools: tools,
			fn: func(ctx context.Context, call llm.ToolCall) (string, error) {
				return "", fmt.Errorf("open %s: no such file or directory", call.Arguments)
			},
		}

		engine := NewEngine(recClient, executor, 0, "test-hallucination")
		res, err := engine.Run(context.Background(), "sys", "user")
		if err != nil {
			t.Fatalf("expected success after 3 strikes steering injection, got err: %v", err)
		}
		if res != "recovered after steering" {
			t.Fatalf("expected 'recovered after steering', got %q", res)
		}

		// Turn 4 received messages should contain the injected steering prompt
		lastTurnMessages := recClient.receivedMessages[len(recClient.receivedMessages)-1]
		foundSteering := false
		for _, msg := range lastTurnMessages {
			if msg.Role == "tool" && strings.Contains(msg.Content, "Guidance: Stop guessing file paths. Use zqk_observer_search first.") {
				foundSteering = true
				break
			}
		}
		if !foundSteering {
			t.Fatal("expected injected steering prompt in tool messages before turn 4")
		}
	})

	t.Run("four_strikes_abort", func(t *testing.T) {
		recClient := &recordingMockClient{
			responses: []llm.StructuredCompletionResponse{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "read_code", Arguments: `{"path":"missing1.go"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "read_code", Arguments: `{"path":"missing2.go"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "read_code", Arguments: `{"path":"missing3.go"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c4", Name: p + "read_code", Arguments: `{"path":"missing4.go"}`}}},
				{Content: "should not be reached"},
			},
		}
		executor := &mockCallExecutor{
			tools: tools,
			fn: func(ctx context.Context, call llm.ToolCall) (string, error) {
				return "", fmt.Errorf("open %s: no such file or directory", call.Arguments)
			},
		}

		engine := NewEngine(recClient, executor, 0, "test-hallucination")
		_, err := engine.Run(context.Background(), "sys", "user")
		if err == nil {
			t.Fatal("expected 4 consecutive file-not-found errors to abort, got nil")
		}
		if !strings.Contains(err.Error(), "hallucination_circuit_breaker: 4 consecutive file-not-found errors") {
			t.Fatalf("expected hallucination circuit breaker abort message, got: %v", err)
		}
	})

	t.Run("interrupted_file_not_found_resets", func(t *testing.T) {
		recClient := &recordingMockClient{
			responses: []llm.StructuredCompletionResponse{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: p + "read_code", Arguments: `{"path":"missing1.go"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c2", Name: p + "read_code", Arguments: `{"path":"missing2.go"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c3", Name: p + "read_code", Arguments: `{"path":"real.go"}`}}},
				{ToolCalls: []llm.ToolCall{{ID: "c4", Name: p + "read_code", Arguments: `{"path":"missing3.go"}`}}},
				{Content: "finished successfully"},
			},
		}
		callCount := 0
		executor := &mockCallExecutor{
			tools: tools,
			fn: func(ctx context.Context, call llm.ToolCall) (string, error) {
				callCount++
				if callCount == 3 {
					return "package real", nil
				}
				return "", fmt.Errorf("open %s: no such file or directory", call.Arguments)
			},
		}

		engine := NewEngine(recClient, executor, 0, "test-hallucination")
		res, err := engine.Run(context.Background(), "sys", "user")
		if err != nil {
			t.Fatalf("expected success when errors are interrupted by real file, got err: %v", err)
		}
		if res != "finished successfully" {
			t.Fatalf("expected 'finished successfully', got %q", res)
		}
	})
}

type mockCallExecutor struct {
	tools []llm.ToolDefinition
	fn    func(ctx context.Context, call llm.ToolCall) (string, error)
}

func (m *mockCallExecutor) GetTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	return m.tools, nil
}

func (m *mockCallExecutor) ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error) {
	if m.fn != nil {
		return m.fn(ctx, call)
	}
	return "ok", nil
}

func (m *mockCallExecutor) Close() error {
	return nil
}

type recordingMockClient struct {
	responses        []llm.StructuredCompletionResponse
	index            int
	receivedMessages [][]llm.Message
}

func (m *recordingMockClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	return "", nil
}

func (m *recordingMockClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (m *recordingMockClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}

func (m *recordingMockClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "", nil
}

func (m *recordingMockClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return "", nil
}

func (m *recordingMockClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return 0.0, nil
}

func (m *recordingMockClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}

func (m *recordingMockClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return "", nil
}

func (m *recordingMockClient) GenerateStructuredCompletion(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (llm.StructuredCompletionResponse, error) {
	m.receivedMessages = append(m.receivedMessages, messages)
	if m.index < len(m.responses) {
		resp := m.responses[m.index]
		m.index++
		return resp, nil
	}
	return llm.StructuredCompletionResponse{Content: "final fallback answer"}, nil
}
