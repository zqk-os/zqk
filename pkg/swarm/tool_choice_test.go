package swarm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestNextToolChoice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   toolChoiceState
		want string
	}{
		{name: "no tools", in: toolChoiceState{}, want: ""},
		{name: "idle auto", in: toolChoiceState{ToolsPresent: true}, want: llm.ToolChoiceAuto},
		{name: "owes work", in: toolChoiceState{ToolsPresent: true, NeedExecutionEvidence: true}, want: llm.ToolChoiceRequired},
		{name: "after markdown", in: toolChoiceState{ToolsPresent: true, MarkdownRecoveries: 1}, want: llm.ToolChoiceRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := nextToolChoice(tc.in); got != tc.want {
				t.Fatalf("nextToolChoice() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextToolChoiceFunction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   toolChoiceState
		want string
	}{
		{name: "pin while evidence owed", in: toolChoiceState{NeedExecutionEvidence: true, PinTool: "zqk_write_code"}, want: "zqk_write_code"},
		{name: "after markdown", in: toolChoiceState{NeedExecutionEvidence: true, PinTool: "zqk_write_code", MarkdownRecoveries: 1}, want: "zqk_write_code"},
		{name: "no pin without owed work", in: toolChoiceState{PinTool: "zqk_write_code", MarkdownRecoveries: 1}, want: ""},
		{name: "evidence met", in: toolChoiceState{NeedExecutionEvidence: false, PinTool: "zqk_write_code"}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := nextToolChoiceFunction(tc.in); got != tc.want {
				t.Fatalf("nextToolChoiceFunction() = %q, want %q", got, tc.want)
			}
		})
	}
}

type recordingClient struct {
	mockClient
	choices []string
	fns     []string
}

func (m *recordingClient) GenerateStructuredCompletionWithOptions(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, opts llm.StructuredCompletionOptions) (llm.StructuredCompletionResponse, error) {
	m.choices = append(m.choices, opts.ToolChoice)
	m.fns = append(m.fns, opts.ToolChoiceFunction)
	return m.GenerateStructuredCompletion(ctx, messages, tools)
}

func TestEngine_toolChoiceRequiredAfterMarkdownRecovery(t *testing.T) {
	markdown := fmt.Sprintf("```json\n{\n  %q: %q,\n  \"arguments\": {}\n}\n```", objects.FieldKeyName, "test_tool")
	client := &recordingClient{
		mockClient: mockClient{
			responses: []llm.StructuredCompletionResponse{
				{Content: markdown},
				{Content: "done after native turn"},
			},
		},
	}
	engine := NewEngine(client, &mockExecutor{}, 0, "test-tool-choice")
	got, err := engine.Run(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "done after native turn" {
		t.Fatalf("result = %q", got)
	}
	if len(client.choices) < 2 {
		t.Fatalf("choices = %#v, want at least 2 turns", client.choices)
	}
	if client.choices[0] != llm.ToolChoiceAuto {
		t.Fatalf("first choice = %q, want auto", client.choices[0])
	}
	if client.choices[1] != llm.ToolChoiceRequired {
		t.Fatalf("second choice = %q, want required after markdown recovery", client.choices[1])
	}
}

func TestEngine_toolChoiceRequiredWhenEvidenceOwed(t *testing.T) {
	client := &recordingClient{
		mockClient: mockClient{
			responses: []llm.StructuredCompletionResponse{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "test_tool", Arguments: "{}"}}},
				{Content: "wrote and verified"},
			},
		},
	}
	engine := NewEngine(client, &mockExecutor{}, 0, "test-evidence").
		RequireSuccessfulToolCalls(1)
	if _, err := engine.Run(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(client.choices) < 1 || client.choices[0] != llm.ToolChoiceRequired {
		t.Fatalf("first choice = %#v, want required while evidence is owed", client.choices)
	}
}

type recordingExecutor struct {
	namedToolsExecutor
	names []string
}

func (m *recordingExecutor) ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error) {
	m.names = append(m.names, call.Name)
	return m.namedToolsExecutor.ExecuteToolCall(ctx, call)
}

func TestEngine_dropsRecoveredLookupWhileWritePinned(t *testing.T) {
	markdownGet := fmt.Sprintf("```json\n{\n  %q: %q,\n  \"arguments\": {}\n}\n```", objects.FieldKeyName, "zqk_object_get")
	exec := &recordingExecutor{namedToolsExecutor: namedToolsExecutor{tools: []string{"zqk_write_code", "zqk_object_get"}}}
	client := &recordingClient{
		mockClient: mockClient{
			responses: []llm.StructuredCompletionResponse{
				{Content: markdownGet},
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "zqk_write_code", Arguments: `{}`}}},
				{Content: "wrote and verified"},
			},
		},
	}
	engine := NewEngine(client, exec, 0, "test-pin-filter").
		RequireAnySuccessfulTools("zqk_write_code")
	if _, err := engine.Run(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, name := range exec.names {
		if name == "zqk_object_get" {
			t.Fatalf("recovered object_get must not execute while write is pinned: %v", exec.names)
		}
	}
	if len(exec.names) == 0 || exec.names[0] != "zqk_write_code" {
		t.Fatalf("executed = %v, want zqk_write_code first", exec.names)
	}
}

type toolsRecordingClient struct {
	recordingClient
	seen [][]string
}

func (m *toolsRecordingClient) GenerateStructuredCompletionWithOptions(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, opts llm.StructuredCompletionOptions) (llm.StructuredCompletionResponse, error) {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	m.seen = append(m.seen, names)
	return m.recordingClient.GenerateStructuredCompletionWithOptions(ctx, messages, tools, opts)
}

func TestEngine_RestrictTools_hidesLookup(t *testing.T) {
	client := &toolsRecordingClient{
		recordingClient: recordingClient{
			mockClient: mockClient{
				responses: []llm.StructuredCompletionResponse{
					{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "zqk_write_code", Arguments: `{}`}}},
					{Content: "wrote"},
				},
			},
		},
	}
	exec := &namedToolsExecutor{tools: []string{"zqk_object_get", "zqk_write_code", "zqk_system_status"}}
	engine := NewEngine(client, exec, 0, "test-restrict").
		RestrictTools("zqk_write_code").
		RequireAnySuccessfulTools("zqk_write_code")
	if _, err := engine.Run(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(client.seen) == 0 {
		t.Fatal("no completions")
	}
	for _, names := range client.seen {
		joined := strings.Join(names, ",")
		if strings.Contains(joined, "object_get") || strings.Contains(joined, "system_status") {
			t.Fatalf("lookup tools leaked onto the wire: %v", names)
		}
	}
}

func TestEngine_dropsNativeLookupAfterUnpaidBudget(t *testing.T) {
	exec := &recordingExecutor{namedToolsExecutor: namedToolsExecutor{
		tools: []string{"zqk_write_code", "zqk_object_get", "zqk_read_code", "zqk_system_status"},
	}}
	responses := make([]llm.StructuredCompletionResponse, 0, unpaidLookupBudget+2)
	for i := 0; i < unpaidLookupBudget; i++ {
		responses = append(responses, llm.StructuredCompletionResponse{
			ToolCalls: []llm.ToolCall{{
				ID:        fmt.Sprintf("g%d", i+1),
				Name:      "zqk_object_get",
				Arguments: toolArgsJSON(map[string]string{objects.FieldKeyID: fmt.Sprintf("obj-%d", i+1)}),
			}},
		})
	}
	responses = append(responses,
		llm.StructuredCompletionResponse{
			ToolCalls: []llm.ToolCall{
				{ID: "overflow_get", Name: "zqk_object_get", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyID: "overflow"})},
				{ID: "w1", Name: "zqk_write_code", Arguments: `{}`},
			},
		},
		llm.StructuredCompletionResponse{Content: "wrote after lookup budget"},
	)

	client := &toolsRecordingClient{
		recordingClient: recordingClient{
			mockClient: mockClient{
				responses: responses,
			},
		},
	}
	engine := NewEngine(client, exec, 0, "test-unpaid-lookup").
		RequireAnySuccessfulTools("zqk_write_code")
	if _, err := engine.Run(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	gets := 0
	for _, name := range exec.names {
		if name == "zqk_object_get" {
			gets++
		}
	}
	if gets != unpaidLookupBudget {
		t.Fatalf("executed object_get = %d (%v), want %d then drop", gets, exec.names, unpaidLookupBudget)
	}
	if len(exec.names) == 0 || exec.names[len(exec.names)-1] != "zqk_write_code" {
		t.Fatalf("executed = %v, want write_code after the budget", exec.names)
	}
	if len(client.seen) < unpaidLookupBudget+1 {
		t.Fatalf("completions = %d, want at least %d", len(client.seen), unpaidLookupBudget+1)
	}
	postBudget := strings.Join(client.seen[unpaidLookupBudget], ",")
	if strings.Contains(postBudget, "object_get") || strings.Contains(postBudget, "system_status") {
		t.Fatalf("lookup tools still on the wire after unpaid budget: %v", client.seen[unpaidLookupBudget])
	}
}

func TestEngine_allowsCodeExplorationBeforeForceWrite(t *testing.T) {
	exec := &recordingExecutor{namedToolsExecutor: namedToolsExecutor{
		tools: []string{"zqk_write_code", "zqk_object_get", "zqk_read_code", "zqk_observer_search"},
	}}
	responses := make([]llm.StructuredCompletionResponse, 0, unpaidLookupBudget+6)
	for i := 0; i < unpaidLookupBudget; i++ {
		responses = append(responses, llm.StructuredCompletionResponse{
			ToolCalls: []llm.ToolCall{{
				ID:        fmt.Sprintf("k%d", i+1),
				Name:      "zqk_object_get",
				Arguments: toolArgsJSON(map[string]string{objects.FieldKeyID: fmt.Sprintf("obj-%d", i+1)}),
			}},
		})
	}
	// After kernel budget, agent performs 4 read_code / observer_search calls.
	// None should be dropped by force-write because streak has not reached unpaidLookupForceWrite.
	for i := 0; i < 4; i++ {
		responses = append(responses, llm.StructuredCompletionResponse{
			ToolCalls: []llm.ToolCall{{
				ID:        fmt.Sprintf("r%d", i+1),
				Name:      "zqk_read_code",
				Arguments: toolArgsJSON(map[string]string{"path": fmt.Sprintf("pkg/foo/bar%d.go", i+1)}),
			}},
		})
	}
	responses = append(responses,
		llm.StructuredCompletionResponse{
			ToolCalls: []llm.ToolCall{
				{ID: "w1", Name: "zqk_write_code", Arguments: `{}`},
			},
		},
		llm.StructuredCompletionResponse{Content: "explored code then wrote"},
	)

	client := &toolsRecordingClient{
		recordingClient: recordingClient{
			mockClient: mockClient{
				responses: responses,
			},
		},
	}
	engine := NewEngine(client, exec, 0, "test-code-exploration").
		RequireAnySuccessfulTools("zqk_write_code")
	if _, err := engine.Run(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	readCount := 0
	for _, name := range exec.names {
		if name == "zqk_read_code" {
			readCount++
		}
	}
	if readCount != 4 {
		t.Fatalf("read_code executed = %d, want 4 (all should be permitted before force write)", readCount)
	}
}

func TestEngine_keepsWriteMenuAfterEvidence(t *testing.T) {
	client := &toolsRecordingClient{
		recordingClient: recordingClient{
			mockClient: mockClient{
				responses: []llm.StructuredCompletionResponse{
					{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "zqk_write_code", Arguments: `{}`}}},
					{Content: "wrote and verified"},
				},
			},
		},
	}
	exec := &namedToolsExecutor{tools: []string{
		"zqk_object_get", "zqk_object_list", "zqk_system_status",
		"zqk_write_code", "zqk_write_file", "zqk_read_code", "zqk_execute_bash",
	}}
	engine := NewEngine(client, exec, 0, "test-post-write-menu").
		RequireAnySuccessfulTools("zqk_write_code")
	if _, err := engine.Run(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(client.seen) < 2 {
		t.Fatalf("completions = %d, want write then summary", len(client.seen))
	}
	after := strings.Join(client.seen[1], ",")
	if strings.Contains(after, "object_get") || strings.Contains(after, "system_status") {
		t.Fatalf("lookup tools returned after write evidence: %v", client.seen[1])
	}
	if !strings.Contains(after, "write_code") || !strings.Contains(after, "execute_bash") {
		t.Fatalf("post-write menu should keep write/test: %v", client.seen[1])
	}
}

func TestEngine_toolChoiceAutoAfterWriteEvidence(t *testing.T) {
	client := &recordingClient{
		mockClient: mockClient{
			responses: []llm.StructuredCompletionResponse{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "zqk_write_code", Arguments: `{}`}}},
				{Content: "wrote and verified"},
			},
		},
	}
	engine := NewEngine(client, &namedToolsExecutor{tools: []string{"zqk_write_code"}}, 0, "test-auto-after-write").
		RequireAnySuccessfulTools("zqk_write_code")
	if _, err := engine.Run(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(client.choices) < 2 {
		t.Fatalf("choices = %#v, want required then auto", client.choices)
	}
	if client.choices[0] != llm.ToolChoiceRequired {
		t.Fatalf("first choice = %q, want required", client.choices[0])
	}
	if client.choices[1] != llm.ToolChoiceAuto {
		t.Fatalf("second choice = %q, want auto after write evidence", client.choices[1])
	}
	if client.fns[1] != "" {
		t.Fatalf("pin after write = %q, want empty", client.fns[1])
	}
}

func TestEngine_pinsMutationToolWhileEvidenceOwed(t *testing.T) {
	client := &recordingClient{
		mockClient: mockClient{
			responses: []llm.StructuredCompletionResponse{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "test_tool", Arguments: `{}`}}},
				{Content: "wrote and verified"},
			},
		},
	}
	engine := NewEngine(client, &mockExecutor{}, 0, "test-pin").
		RequireAnySuccessfulTools("test_tool")
	if _, err := engine.Run(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(client.fns) < 1 || client.fns[0] != "test_tool" {
		t.Fatalf("first pin = %#v, want test_tool on the first owed turn", client.fns)
	}
}

func TestCheckReadLoopWatchdog(t *testing.T) {
	t.Parallel()

	t.Run("empty history", func(t *testing.T) {
		t.Parallel()
		tripped, reason := CheckReadLoopWatchdog(nil)
		if tripped || reason != "" {
			t.Fatalf("expected false on empty history, got %v (%s)", tripped, reason)
		}
	})

	t.Run("repeated identical arguments trip watchdog", func(t *testing.T) {
		t.Parallel()
		history := []ToolCallRecord{
			{Name: "zqk_read_code", Arguments: `{"path":"pkg/swarm/engine.go"}`},
			{Name: "zqk_read_code", Arguments: `{"path":"pkg/swarm/engine.go"}`},
			{Name: "zqk_read_code", Arguments: `{"path":"pkg/swarm/engine.go"}`},
		}
		tripped, reason := CheckReadLoopWatchdog(history)
		if !tripped {
			t.Fatal("expected watchdog to trip on 3 repeated identical calls")
		}
		if !strings.Contains(reason, "Technical Lead Watchdog") {
			t.Fatalf("expected Technical Lead Watchdog in reason, got: %s", reason)
		}
		if !ShouldForceWriteOnly(true, history) {
			t.Fatal("expected ShouldForceWriteOnly to return true when watchdog trips")
		}
	})

	t.Run("non-mutation streak threshold trips watchdog", func(t *testing.T) {
		t.Parallel()
		var history []ToolCallRecord
		for i := 0; i < ReadLoopWatchdogThreshold; i++ {
			history = append(history, ToolCallRecord{
				Name:      "zqk_read_code",
				Arguments: fmt.Sprintf(`{"path":"file_%d.go"}`, i),
			})
		}
		tripped, reason := CheckReadLoopWatchdog(history)
		if !tripped {
			t.Fatalf("expected watchdog to trip after %d non-mutation calls", ReadLoopWatchdogThreshold)
		}
		if !strings.Contains(reason, "streak reached") {
			t.Fatalf("expected streak reached in reason, got: %s", reason)
		}
		if !ShouldForceWriteOnly(true, history) {
			t.Fatal("expected ShouldForceWriteOnly to return true on streak threshold")
		}
	})

	t.Run("mutation tool resets streak", func(t *testing.T) {
		t.Parallel()
		history := []ToolCallRecord{
			{Name: "zqk_read_code", Arguments: `{"path":"pkg/swarm/engine.go"}`},
			{Name: "zqk_read_code", Arguments: `{"path":"pkg/swarm/engine.go"}`},
			{Name: "zqk_write_code", Arguments: `{"path":"pkg/swarm/engine.go","content":"package swarm"}`},
		}
		tripped, _ := CheckReadLoopWatchdog(history)
		if tripped {
			t.Fatal("expected watchdog not to trip after mutation tool")
		}
		if ShouldForceWriteOnly(true, history) {
			t.Fatal("expected ShouldForceWriteOnly false after mutation tool")
		}
	})
}
