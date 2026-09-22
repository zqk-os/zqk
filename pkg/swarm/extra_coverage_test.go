package swarm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestTokenTracker_DefaultsAndEdgeCases(t *testing.T) {
	t.Parallel()

	// Default fallbacks when <= 0
	tracker := NewTokenTracker(0, 0)
	if tracker.ContextWindow != 32768 {
		t.Errorf("expected 32768 default context window, got %d", tracker.ContextWindow)
	}
	if tracker.Threshold != 0.9 {
		t.Errorf("expected 0.9 default threshold, got %f", tracker.Threshold)
	}

	// Threshold > 1.0 fallback
	tracker2 := NewTokenTracker(1000, 1.5)
	if tracker2.Threshold != 0.9 {
		t.Errorf("expected 0.9 default threshold when > 1.0, got %f", tracker2.Threshold)
	}

	// EstimateText edge cases
	if n := tracker.EstimateText(""); n != 0 {
		t.Errorf("expected 0 tokens for empty string, got %d", n)
	}
	if n := tracker.EstimateText("a"); n != 1 {
		t.Errorf("expected 1 token for single char, got %d", n)
	}
}

func TestCompressSchema(t *testing.T) {
	t.Parallel()

	// nil schema
	compressSchema(nil)

	// schema with non-essential fields, properties, and items
	schema := map[string]any{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"format":               "uri",
		"default":              "val",
		"examples":             []any{"ex1"},
		"additionalProperties": false,
		"patternProperties":    map[string]any{},
		"keep":                 "important",
		"properties": map[string]any{
			"nested": map[string]any{
				"examples": []any{"nested_ex"},
				"default":  "nested_val",
				"type":     "string",
			},
		},
		"items": map[string]any{
			"format": "email",
			"type":   "string",
		},
	}

	compressSchema(schema)

	if _, ok := schema["$schema"]; ok {
		t.Errorf("expected $schema to be deleted")
	}
	if _, ok := schema["format"]; ok {
		t.Errorf("expected format to be deleted")
	}
	if _, ok := schema["examples"]; ok {
		t.Errorf("expected examples to be deleted")
	}
	if schema["keep"] != "important" {
		t.Errorf("expected keep to be preserved")
	}

	props := schema["properties"].(map[string]any)
	nested := props["nested"].(map[string]any)
	if _, ok := nested["examples"]; ok {
		t.Errorf("expected nested examples to be deleted")
	}
	if nested["type"] != "string" {
		t.Errorf("expected nested type to be preserved")
	}

	items := schema["items"].(map[string]any)
	if _, ok := items["format"]; ok {
		t.Errorf("expected items format to be deleted")
	}
}

func TestLevenshteinDistance(t *testing.T) {
	t.Parallel()

	if d := levenshteinDistance("", ""); d != 0 {
		t.Errorf("expected 0 for both empty, got %d", d)
	}
	if d := levenshteinDistance("abc", ""); d != 3 {
		t.Errorf("expected 3 for empty s2, got %d", d)
	}
	if d := levenshteinDistance("", "abcd"); d != 4 {
		t.Errorf("expected 4 for empty s1, got %d", d)
	}
	if d := levenshteinDistance("kitten", "sitting"); d != 3 {
		t.Errorf("expected 3 for kitten/sitting, got %d", d)
	}
	if d := levenshteinDistance("same", "same"); d != 0 {
		t.Errorf("expected 0 for identical strings, got %d", d)
	}
}

func TestPrompts_ContextGatheringLoopGuidance(t *testing.T) {
	t.Parallel()

	guidance := ContextGatheringLoopGuidance()
	if !strings.Contains(guidance, "WARNING") {
		t.Errorf("expected guidance to contain WARNING, got %q", guidance)
	}
}

func TestGetProjectRoot(t *testing.T) {
	t.Parallel()
	root := getProjectRoot()
	// Should resolve current repo root or non-empty string in repo
	if root == "" {
		t.Log("getProjectRoot returned empty string outside git root")
	}
}

func TestToolGuard_EdgeCases(t *testing.T) {
	t.Parallel()

	// objectListLimit variations
	if _, ok := objectListLimit(nil); ok {
		t.Errorf("expected false for nil args")
	}
	if _, ok := objectListLimit(map[string]any{"limit": nil}); ok {
		t.Errorf("expected false for nil limit")
	}
	if n, ok := objectListLimit(map[string]any{"limit": float64(12)}); !ok || n != 12 {
		t.Errorf("expected 12, true for float64, got %d, %v", n, ok)
	}
	if n, ok := objectListLimit(map[string]any{"limit": int(7)}); !ok || n != 7 {
		t.Errorf("expected 7, true for int, got %d, %v", n, ok)
	}
	if n, ok := objectListLimit(map[string]any{"limit": "15"}); !ok || n != 15 {
		t.Errorf("expected 15, true for string, got %d, %v", n, ok)
	}
	if _, ok := objectListLimit(map[string]any{"limit": "not_an_int"}); ok {
		t.Errorf("expected false for invalid string limit")
	}
	if _, ok := objectListLimit(map[string]any{"limit": true}); ok {
		t.Errorf("expected false for boolean limit")
	}

	// placeholderObjectID
	if placeholderObjectID("") {
		t.Errorf("expected false for empty arguments")
	}
	if !placeholderObjectID(`{"id": "<ID>"}`) {
		t.Errorf("expected true for <ID>")
	}
	if !placeholderObjectID(`{"object_id": "OBJECT_ID"}`) {
		t.Errorf("expected true for OBJECT_ID")
	}
	if !placeholderObjectID(`{"id": "ID"}`) {
		t.Errorf("expected true for ID")
	}
	if placeholderObjectID(`{"id": "BLI-12345"}`) {
		t.Errorf("expected false for valid ID BLI-12345")
	}

	// unscopedObjectList
	if !unscopedObjectList("") {
		t.Errorf("expected true for empty arguments")
	}
	if !unscopedObjectList(`{"kind": ""}`) {
		t.Errorf("expected true for empty kind")
	}
	if unscopedObjectList(`{"kind": "backlog_item"}`) {
		t.Errorf("expected false for populated kind")
	}

	// mcpCallInnerIsNotATool
	if !mcpCallInnerIsNotATool("") {
		t.Errorf("expected true for empty arguments")
	}
	if !mcpCallInnerIsNotATool(`{"tool_name": "run bash command"}`) {
		t.Errorf("expected true for tool name with spaces")
	}
	if mcpCallInnerIsNotATool(`{"tool_name": "valid_tool"}`) {
		t.Errorf("expected false for valid tool name")
	}

	// stringifyArg
	if s := stringifyArg("hello"); s != "hello" {
		t.Errorf("expected hello, got %q", s)
	}
	if s := stringifyArg(123); s != "" {
		t.Errorf("expected empty string for non-stringer int, got %q", s)
	}
}

func TestPrompts_RenderPromptEdgeCases(t *testing.T) {
	t.Parallel()

	// RenderSystemPrompt docs_eval and default prefix
	sysPrompt, err := RenderSystemPrompt(QwenSystemData{
		WorkClass: "docs_eval",
	})
	if err != nil {
		t.Fatalf("RenderSystemPrompt failed: %v", err)
	}
	if sysPrompt == "" {
		t.Errorf("expected non-empty docs_eval system prompt")
	}

	// RenderTaskPrompt default prefix
	taskPrompt, err := RenderTaskPrompt(QwenTaskData{
		TaskName: "TASK-1",
	})
	if err != nil {
		t.Fatalf("RenderTaskPrompt failed: %v", err)
	}
	if taskPrompt == "" {
		t.Errorf("expected non-empty task prompt")
	}
}

func TestEngine_HooksAndConfiguration(t *testing.T) {
	t.Parallel()

	engine := NewEngine(nil, nil, 0, "test-engine")
	defaultHookCount := len(engine.Hooks())

	// WithHooks
	dummyHook := &ProactiveWorkspaceSeeder{}
	engine.WithHooks(dummyHook)
	if len(engine.Hooks()) != defaultHookCount+1 {
		t.Errorf("expected %d hooks after WithHooks, got %d", defaultHookCount+1, len(engine.Hooks()))
	}

	// SetHooks
	engine.SetHooks(dummyHook)
	if len(engine.Hooks()) != 1 {
		t.Errorf("expected 1 hook after SetHooks, got %d", len(engine.Hooks()))
	}

	// WithMaxSteps
	engine.WithMaxSteps(42)
	if engine.maxSteps != 42 {
		t.Errorf("expected maxSteps=42, got %d", engine.maxSteps)
	}

	// RequireSuccessfulToolCalls and RequireAnySuccessfulTools
	engine.RequireSuccessfulToolCalls(5)
	if engine.minSuccessfulToolCalls != 5 {
		t.Errorf("expected minSuccessfulToolCalls=5, got %d", engine.minSuccessfulToolCalls)
	}
	engine.RequireAnySuccessfulTools(" write code ", "execute_bash")
	if len(engine.requiredAnySuccessfulTools) != 2 {
		t.Errorf("expected 2 requiredAnySuccessfulTools, got %d", len(engine.requiredAnySuccessfulTools))
	}
}

func TestHooks_UncoveredMethods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	call := llm.ToolCall{Name: "zqk_write_code"}

	// ToolDenialGuard OnSuccess
	tdg := NewToolDenialGuard()
	tdg.OnSuccess(ctx, call, "result")

	// HallucinationCircuitBreaker OnSuccess
	hcb := NewHallucinationCircuitBreaker()
	hcb.OnSuccess(ctx, call, "result")

	// ProactiveWorkspaceSeeder PreTool, PostTool, OnSuccess
	pws := &ProactiveWorkspaceSeeder{}
	if err := pws.PreTool(ctx, call); err != nil {
		t.Errorf("expected nil error from PreTool")
	}
	if out, err := pws.PostTool(ctx, call, "res", nil); out != "" || err != nil {
		t.Errorf("unexpected PostTool output: %q, %v", out, err)
	}
	pws.OnSuccess(ctx, call, "ok")
	pws.OnSuccess(ctx, llm.ToolCall{Name: "zqk_execute_bash"}, "ok")
	pws.OnSuccess(ctx, llm.ToolCall{Name: "other_tool"}, "ok")
}

func TestLLMTrace_FormatAndSanitize(t *testing.T) {
	t.Parallel()

	// sanitizeTraceID
	if id := sanitizeTraceID(""); id != "unknown" {
		t.Errorf("expected unknown for empty id, got %q", id)
	}
	if id := sanitizeTraceID("engine/task:123@#"); id != "engine_task_123__" {
		t.Errorf("unexpected sanitized id: %q", id)
	}

	// formatLLMTrace
	rec := llmTraceRecord{
		EngineID:           "test-eng",
		Step:               1,
		ToolChoice:         "auto",
		ToolChoiceFunction: "write_code",
		ToolNames:          []string{"read_code", "write_code"},
		DroppedToolNames:   []string{"bash"},
		RawContent:         "```json\n{\"tool\": \"write_code\"}\n```",
		Prompt: []llm.Message{
			{
				Role:       "user",
				Name:       "alice",
				ToolCallID: "tc-1",
				Content:    "please write code\n",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "call-1",
						Name: "read_code",
					},
				},
			},
			{
				Role:    "assistant",
				Content: "",
			},
		},
	}
	formatted := formatLLMTrace(rec)
	if !strings.Contains(formatted, "test-eng") || !strings.Contains(formatted, "pin_filter_dropped: bash") {
		t.Errorf("unexpected formatLLMTrace output: %q", formatted)
	}
}

func TestToolPathArg(t *testing.T) {
	t.Parallel()

	if p := toolPathArg(""); p != "" {
		t.Errorf("expected empty string for empty arguments, got %q", p)
	}
	if p := toolPathArg(`{"path": "pkg/swarm/file.go"}`); p != "pkg/swarm/file.go" {
		t.Errorf("expected pkg/swarm/file.go, got %q", p)
	}
}

func TestTokenTracker_CompactHistory_ExceedsBudget(t *testing.T) {
	t.Parallel()

	tracker := NewTokenTracker(100, 0.5) // budget is 50 tokens

	// 1. messages <= 2
	msgs2 := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "user"},
	}
	if _, ok := tracker.CompactHistory(msgs2, nil); ok {
		t.Errorf("expected false when len <= 2")
	}

	// 2. messages > 2 and exceeds budget
	largeContent := strings.Repeat("huge content block to exceed budget ", 20)
	msgsMany := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "task"},
		{Role: "assistant", Content: largeContent},
		{Role: "user", Content: largeContent},
		{Role: "assistant", Content: largeContent},
		{Role: "user", Content: "final prompt"},
	}
	compacted, _ := tracker.CompactHistory(msgsMany, nil)
	if len(compacted) >= len(msgsMany) {
		t.Errorf("expected history compaction, got %d messages", len(compacted))
	}
}

func TestInMemoryTaskQueue_Errors(t *testing.T) {
	t.Parallel()

	q := NewInMemoryTaskQueue()

	// Canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.Enqueue(ctx, Task{ID: "t1"}); err == nil {
		t.Errorf("expected error on canceled context in Enqueue")
	}

	// Closed queue
	q.Close()
	if err := q.Enqueue(context.Background(), Task{ID: "t2"}); err == nil {
		t.Errorf("expected error on closed queue in Enqueue")
	}
}

func TestObjectUpdateSetsStatus(t *testing.T) {
	t.Parallel()

	if objectUpdateSetsStatus("") {
		t.Errorf("expected false for empty arguments")
	}
	if !objectUpdateSetsStatus(`{"status": "complete"}`) {
		t.Errorf("expected true for direct status field")
	}
	if !objectUpdateSetsStatus(`{"fields": ["status=complete", "title=New"]}`) {
		t.Errorf("expected true for status in fields array")
	}
	if objectUpdateSetsStatus(`{"fields": ["title=New", "priority=high"]}`) {
		t.Errorf("expected false for non-status fields")
	}
	if !objectUpdateSetsStatus(`{"updates": {"status": "in_progress"}}`) {
		t.Errorf("expected true for status in updates map")
	}
	if objectUpdateSetsStatus(`{"updates": {"title": "Updated"}}`) {
		t.Errorf("expected false for updates map without status")
	}
}

func TestToolGuard_InventedAndCatalog(t *testing.T) {
	t.Parallel()

	// inventedRepoPath
	if inventedRepoPath("") {
		t.Errorf("expected false for empty path")
	}
	if !inventedRepoPath("BLI-12345.go") {
		t.Errorf("expected true for kernel object ID path")
	}
	if !inventedRepoPath("yourrepo/main.go") {
		t.Errorf("expected true for tutorial yourrepo path")
	}
	if !inventedRepoPath("helloworld/main.go") {
		t.Errorf("expected true for tutorial helloworld path")
	}
	if !inventedRepoPath("src/main.go") {
		t.Errorf("expected true for tutorial src/main.go path")
	}
	if inventedRepoPath("pkg/swarm/tool_guard.go") {
		t.Errorf("expected false for real path")
	}

	// isInventedMutationTool
	if !isInventedMutationTool("zqk_replace_code") {
		t.Errorf("expected true for replace_code")
	}
	if !isInventedMutationTool("zqk_commit") {
		t.Errorf("expected true for commit")
	}

	// isMCPCatalogTool and MCPCatalogToolGuidance
	if !isMCPCatalogTool("zqk_mcp_list_tools") {
		t.Errorf("expected true for mcp_list_tools")
	}
	if !isMCPCatalogTool("zqk_get_tool_schema") {
		t.Errorf("expected true for get_tool_schema")
	}
	if isMCPCatalogTool("zqk_read_file") {
		t.Errorf("expected false for read_file")
	}
	if guidance := MCPCatalogToolGuidance(); guidance == "" {
		t.Errorf("expected non-empty MCPCatalogToolGuidance")
	}
}

func TestToolGuard_AllGuidanceAndPredicates(t *testing.T) {
	t.Parallel()

	// Predicates
	if !isHourglassOnlyTool("agent_next") {
		t.Errorf("expected true for agent_next")
	}
	if !isObjectUpdateTool("zqk_object_update") {
		t.Errorf("expected true for zqk_object_update")
	}
	if !isObjectGetTool("zqk_object_get") {
		t.Errorf("expected true for zqk_object_get")
	}
	if !isObjectListTool("zqk_object_list") {
		t.Errorf("expected true for zqk_object_list")
	}

	// Guidance functions
	if g := InventedMutationToolGuidance(); g == "" {
		t.Errorf("expected non-empty InventedMutationToolGuidance")
	}
	if g := HourglassOnlyToolGuidance(); g == "" {
		t.Errorf("expected non-empty HourglassOnlyToolGuidance")
	}
	if g := StatusFieldUpdateGuidance(); g == "" {
		t.Errorf("expected non-empty StatusFieldUpdateGuidance")
	}
	if g := PlaceholderObjectIDGuidance(); g == "" {
		t.Errorf("expected non-empty PlaceholderObjectIDGuidance")
	}
	if g := OversizedObjectListGuidance(); g == "" {
		t.Errorf("expected non-empty OversizedObjectListGuidance")
	}
	if g := UnscopedObjectListGuidance(); g == "" {
		t.Errorf("expected non-empty UnscopedObjectListGuidance")
	}
	if g := InventedObjectListKindGuidance(); g == "" {
		t.Errorf("expected non-empty InventedObjectListKindGuidance")
	}
	if g := MCPCallInnerNotAToolGuidance("bogus_call"); g == "" {
		t.Errorf("expected non-empty MCPCallInnerNotAToolGuidance")
	}
}

func TestFeedbackProcessor_Process(t *testing.T) {
	t.Parallel()

	// 1. Success case and retry case and max retries case
	q := NewInMemoryTaskQueue()
	fp := NewFeedbackProcessor(q)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan TaskResult, 4)
	// Success
	in <- TaskResult{Task: Task{ID: "t-success"}, Result: "ok"}
	// Retryable error
	in <- TaskResult{Task: Task{ID: "t-retry", MaxRetries: 2, CurrentRetries: 0}, Error: errors.New("temporary failure")}
	// Max retries reached error
	in <- TaskResult{Task: Task{ID: "t-max", MaxRetries: 1, CurrentRetries: 1}, Error: errors.New("terminal failure")}
	// Canceled error
	in <- TaskResult{Task: Task{ID: "t-cancel"}, Error: context.Canceled}
	close(in)

	out := fp.Process(ctx, in)
	var received []TaskResult
	for res := range out {
		received = append(received, res)
	}

	// Should have received 3 results (success, max-retried, canceled). The retry was pushed back to queue.
	if len(received) != 3 {
		t.Fatalf("expected 3 results from processor, got %d", len(received))
	}

	// Verify the retried task was enqueued
	retried, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("expected to dequeue retried task: %v", err)
	}
	if retried.ID != "t-retry" || retried.CurrentRetries != 1 {
		t.Errorf("unexpected retried task: %+v", retried)
	}
}

func TestFeedbackProcessor_Process_EnqueueFailure(t *testing.T) {
	t.Parallel()

	// Closed queue causes Enqueue to fail during retry
	q := NewInMemoryTaskQueue()
	q.Close()
	fp := NewFeedbackProcessor(q)

	ctx := context.Background()
	in := make(chan TaskResult, 1)
	in <- TaskResult{Task: Task{ID: "t-fail", MaxRetries: 2, CurrentRetries: 0}, Error: errors.New("some error")}
	close(in)

	out := fp.Process(ctx, in)
	var received []TaskResult
	for res := range out {
		received = append(received, res)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 result after enqueue failure, got %d", len(received))
	}
}

func TestEngine_VerifyCompletionWith(t *testing.T) {
	t.Parallel()

	engine := NewEngine(nil, nil, 0, "test")
	// nil verifier
	if res := engine.VerifyCompletionWith(nil, 5); res != engine {
		t.Errorf("expected engine return")
	}

	// valid verifier with negative repairs
	verifier := func(ctx context.Context, history []ToolCallRecord) (string, error) {
		return "", nil
	}
	engine.VerifyCompletionWith(verifier, -2)
	if engine.maxCompletionRepairs != 0 {
		t.Errorf("expected 0 max repairs, got %d", engine.maxCompletionRepairs)
	}

	// positive repairs
	engine.VerifyCompletionWith(verifier, 3)
	if engine.maxCompletionRepairs != 3 {
		t.Errorf("expected 3 max repairs, got %d", engine.maxCompletionRepairs)
	}
}

func TestDetectRepeatingPattern(t *testing.T) {
	t.Parallel()

	// Short history
	if detected, _ := detectRepeatingPattern(nil); detected {
		t.Errorf("expected false for nil history")
	}

	// Repeating pattern without soft blocked
	recA := ToolCallRecord{Name: "tool_a", Arguments: "arg1"}
	history := []ToolCallRecord{recA, recA, recA}
	if detected, _ := detectRepeatingPattern(history); detected {
		t.Errorf("expected false when no soft blocked")
	}

	// Repeating pattern with soft blocked
	recASoft := ToolCallRecord{Name: "tool_a", Arguments: "arg1", SoftBlocked: true}
	historyBlocked := []ToolCallRecord{recA, recASoft, recA}
	if detected, desc := detectRepeatingPattern(historyBlocked); !detected || !strings.Contains(desc, "tool_a") {
		t.Errorf("expected true with tool_a, got %v, %q", detected, desc)
	}

	// Period 2 pattern with soft blocked
	recB := ToolCallRecord{Name: "tool_b", Arguments: "arg2"}
	recBSoft := ToolCallRecord{Name: "tool_b", Arguments: "arg2", SoftBlocked: true}
	historyP2 := []ToolCallRecord{
		recA, recB,
		recA, recBSoft,
		recA, recB,
	}
	if detected, desc := detectRepeatingPattern(historyP2); !detected || !strings.Contains(desc, "tool_a tool_b") {
		t.Errorf("expected true with period 2, got %v, %q", detected, desc)
	}
}

func TestLLMTrace_ChannelAndHint(t *testing.T) {
	t.Parallel()

	// traceChannel branches
	if c := traceChannel(llmTraceRecord{NativeToolCalls: []llm.ToolCall{{Name: "call"}}}); c != "native" {
		t.Errorf("expected native, got %s", c)
	}
	if c := traceChannel(llmTraceRecord{RecoveredMarkdown: true, ExecutedToolCalls: []llm.ToolCall{{Name: "call"}}}); c != "markdown-recovered" {
		t.Errorf("expected markdown-recovered, got %s", c)
	}
	if c := traceChannel(llmTraceRecord{RecoveredMarkdown: true}); c != "markdown-recovered-dropped" {
		t.Errorf("expected markdown-recovered-dropped, got %s", c)
	}
	if c := traceChannel(llmTraceRecord{RawContent: "   "}); c != "empty" {
		t.Errorf("expected empty, got %s", c)
	}
	if c := traceChannel(llmTraceRecord{RawContent: "some text"}); c != "text" {
		t.Errorf("expected text, got %s", c)
	}

	// toolChannelHint
	if h := toolChannelHint("content", 1); h != "" {
		t.Errorf("expected empty hint when nativeCount > 0, got %q", h)
	}
	if h := toolChannelHint("<tool_call>", 0); !strings.Contains(h, "Hermes") {
		t.Errorf("expected Hermes hint, got %q", h)
	}
	if h := toolChannelHint("```json\n", 0); !strings.Contains(h, "markdown JSON") {
		t.Errorf("expected markdown hint, got %q", h)
	}
	if h := toolChannelHint("plain answer", 0); h != "" {
		t.Errorf("expected empty hint for plain answer, got %q", h)
	}

	// toolDefinitionNames
	tools := []llm.ToolDefinition{
		{Name: " tool1 "},
		{Name: ""},
		{Name: "tool2"},
	}
	names := toolDefinitionNames(tools)
	if len(names) != 2 || names[0] != "tool1" || names[1] != "tool2" {
		t.Errorf("unexpected toolDefinitionNames: %v", names)
	}
}

type extraStringer struct{}

func (extraStringer) String() string { return "s" }

func TestExtraGuardsTraceAndArgs(t *testing.T) {
	ctx := context.Background()
	call := llm.ToolCall{Name: "write_code"}
	g := NewToolDenialGuard()
	g.OnSuccess(ctx, call, "ok")
	if _, err := g.PostTool(ctx, call, "ALLOWLIST DENY", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := g.PostTool(ctx, call, "Soft-blocked", errors.New("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.PostTool(ctx, call, "GUIDANCE: Do not use", nil); err == nil {
		t.Fatal("expected denial abort")
	}
	if _, err := g.PostTool(ctx, call, "ok", nil); err != nil {
		t.Fatal(err)
	}
	h := NewHallucinationCircuitBreaker()
	h.OnSuccess(ctx, call, "ok")
	if _, err := h.PostTool(ctx, call, "No such file or directory", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PostTool(ctx, call, "no such file or directory", nil); err != nil {
		t.Fatal(err)
	}
	out, err := h.PostTool(ctx, call, "NO SUCH FILE OR DIRECTORY", nil)
	if err != nil || !strings.Contains(out, "zqk_observer_search") {
		t.Fatalf("guidance=%q err=%v", out, err)
	}
	if _, err := h.PostTool(ctx, call, "no such file or directory", nil); err == nil {
		t.Fatal("expected hallucination abort")
	}
	if _, err := h.PostTool(ctx, call, "fine", nil); err != nil {
		t.Fatal(err)
	}
	seeder := &ProactiveWorkspaceSeeder{}
	if err := seeder.PreTool(ctx, call); err != nil {
		t.Fatal(err)
	}
	if _, err := seeder.PostTool(ctx, call, "ok", nil); err != nil {
		t.Fatal(err)
	}
	seeder.OnSuccess(ctx, call, "ok")
	seeder.OnSuccess(ctx, llm.ToolCall{Name: "execute_bash"}, "ok")
	if stringifyArg("x") != "x" || stringifyArg(extraStringer{}) != "s" || stringifyArg(1) != "" {
		t.Fatal("stringifyArg")
	}
	if parseToolArgs("") != nil || parseToolArgs("{") != nil {
		t.Fatal("parseToolArgs")
	}
	_ = parseToolArgs(`{"k":"v"}`)
	_ = mcpCallInnerIsNotATool("")
	_ = mcpCallInnerIsNotATool(`{"name":"inner tool"}`)
	_ = mcpCallInnerIsNotATool(`{"name":"ok"}`)
	dir := t.TempDir()
	t.Setenv(zqkenv.LLMTrace().Name(), "1")
	t.Setenv(zqkenv.LLMTraceDir().Name(), dir)
	if !llmTraceEnabled() {
		t.Fatal("trace enabled")
	}
	if llmTraceDir() != dir {
		t.Fatalf("dir=%q", llmTraceDir())
	}
	writeLLMTrace(llmTraceRecord{
		EngineID:   "trace id!",
		Step:       1,
		Prompt:     []llm.Message{{Role: "user", Name: "n", Content: "hi"}},
		RawContent: "<tool_call>",
	})
	_ = getProjectRoot()
	t.Setenv(zqkenv.ProjectRoot().Name(), dir)
	_ = getProjectRoot()
	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), dir)
	_ = getProjectRoot()
	ex := &MCPExecutor{}
	ex.SetTelemetryContext("persona", nil)
	if _, err := NewMCPExecutor(ctx, ""); err == nil {
		t.Fatal("empty mcp path")
	}
	_, _ = clipSwarmToolResult("object_list", strings.Repeat("x", swarmListResultCap+1))
	_ = swarmToolResultCap("read_file")
	_ = TruncatedToolResultGuidance()
	_ = fmt.Sprint(sanitizeTraceID(""))
}
