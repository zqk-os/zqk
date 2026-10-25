// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package swarm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/telemetry"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// mockMCPTransport implements mcp.Transport for deterministic in-memory testing.
type mockMCPTransport struct {
	mu       sync.Mutex
	readResp []byte
}

func (m *mockMCPTransport) ReadMessage(r *bufio.Reader) ([]byte, *mcp.MessageFormat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.readResp) == 0 {
		return nil, nil, io.EOF
	}
	resp := m.readResp
	return resp, &mcp.MessageFormat{IsRawJSON: true}, nil
}

func (m *mockMCPTransport) WriteMessage(w *bufio.Writer, data []byte, format *mcp.MessageFormat) error {
	return nil
}

// mockPromptStorageProvider implements storage.ObjectStorageProvider for prompt template tests.
type mockPromptStorageProvider struct {
	storage.ObjectStorageProvider
	readFunc func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
}

func (m *mockPromptStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readFunc != nil {
		return m.readFunc(ctx, secCtx, id)
	}
	return nil, errors.New("not found")
}

// mockElevationExecutor implements Executor with customizable tools and execute handlers.
type mockElevationExecutor struct {
	getToolsFunc func(ctx context.Context) ([]llm.ToolDefinition, error)
	tools        []llm.ToolDefinition
	toolsErr     error
	executeErr   error
	result       string
}

func (m *mockElevationExecutor) GetTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	if m.getToolsFunc != nil {
		return m.getToolsFunc(ctx)
	}
	if m.toolsErr != nil {
		return nil, m.toolsErr
	}
	if len(m.tools) > 0 {
		return m.tools, nil
	}
	return []llm.ToolDefinition{{Name: "zqk_read_file"}}, nil
}

func (m *mockElevationExecutor) ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error) {
	if m.executeErr != nil {
		return "", m.executeErr
	}
	if m.result != "" {
		return m.result, nil
	}
	return "mock result", nil
}

func (m *mockElevationExecutor) Close() error {
	return nil
}

// mockElevationClient implements llm.Client for structured completions.
type mockElevationClient struct {
	responses []llm.StructuredCompletionResponse
	index     int
	err       error
}

func (m *mockElevationClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	return "", nil
}

func (m *mockElevationClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (m *mockElevationClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}

func (m *mockElevationClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "", nil
}

func (m *mockElevationClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return "", nil
}

func (m *mockElevationClient) SemanticCompare(ctx context.Context, obs string, exp string) (float64, error) {
	return 0.0, nil
}

func (m *mockElevationClient) VerifyImage(ctx context.Context, img []byte, ref []byte, prompt string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}

func (m *mockElevationClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return "", nil
}

func (m *mockElevationClient) GenerateStructuredCompletion(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (llm.StructuredCompletionResponse, error) {
	if m.err != nil {
		return llm.StructuredCompletionResponse{}, m.err
	}
	if m.index < len(m.responses) {
		resp := m.responses[m.index]
		m.index++
		return resp, nil
	}
	return llm.StructuredCompletionResponse{Content: "default completion"}, nil
}

// mockSwarmRunner implements Runner for dispatcher tests.
type mockSwarmRunner struct {
	runFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

func (m *mockSwarmRunner) Run(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, systemPrompt, userPrompt)
	}
	return "ok", nil
}

// mockSwarmHook implements AgentHook for engine hook tests.
type mockSwarmHook struct {
	preToolErr  error
	postToolErr error
	steerPrompt string
}

func (m *mockSwarmHook) PreTool(ctx context.Context, call llm.ToolCall) error {
	return m.preToolErr
}

func (m *mockSwarmHook) PostTool(ctx context.Context, call llm.ToolCall, result string, err error) (string, error) {
	return m.steerPrompt, m.postToolErr
}

func (m *mockSwarmHook) OnSuccess(ctx context.Context, call llm.ToolCall, result string) {}

func TestSwarmEngine_HelpersAndOptions(t *testing.T) {
	t.Parallel()

	// Verify package internal min helper
	if min(2, 5) != 2 {
		t.Errorf("expected min(2, 5) == 2, got %d", min(2, 5))
	}
	if min(10, 3) != 3 {
		t.Errorf("expected min(10, 3) == 3, got %d", min(10, 3))
	}

	exec := &mockElevationExecutor{tools: []llm.ToolDefinition{{Name: "zqk_read_file"}}}
	engine := NewEngine(nil, exec, 1024, "eng-c218")

	// Verify hooks configuration
	initialHooks := engine.Hooks()
	if len(initialHooks) == 0 {
		t.Errorf("expected default hooks to be set")
	}

	customHook := &mockSwarmHook{}
	engine.WithHooks(customHook)
	if len(engine.Hooks()) != len(initialHooks)+1 {
		t.Errorf("expected hooks length %d, got %d", len(initialHooks)+1, len(engine.Hooks()))
	}

	engine.SetHooks(customHook)
	if len(engine.Hooks()) != 1 {
		t.Errorf("expected 1 hook after SetHooks, got %d", len(engine.Hooks()))
	}

	// Verify WithMaxSteps and WithExtraSystem
	engine.WithMaxSteps(15)
	if engine.maxSteps != 15 {
		t.Errorf("expected maxSteps 15, got %d", engine.maxSteps)
	}
	engine.WithMaxSteps(-1) // should not decrease below 1
	if engine.maxSteps != 15 {
		t.Errorf("expected maxSteps to stay 15, got %d", engine.maxSteps)
	}

	engine.WithExtraSystem("  always emit JSON  ")
	if engine.extraSystem != "always emit JSON" {
		t.Errorf("expected trimmed extraSystem, got %q", engine.extraSystem)
	}
}

func TestSwarmEngine_RequireSuccessfulToolCallsOptions(t *testing.T) {
	t.Parallel()

	exec := &mockElevationExecutor{tools: []llm.ToolDefinition{{Name: "zqk_read_file"}}}
	engine := NewEngine(nil, exec, 1024, "eng-evidence")

	engine.RequireSuccessfulToolCalls(0)
	if engine.minSuccessfulToolCalls != 0 {
		t.Errorf("expected 0 for non-positive min, got %d", engine.minSuccessfulToolCalls)
	}
	engine.RequireSuccessfulToolCalls(4)
	if engine.minSuccessfulToolCalls != 4 {
		t.Errorf("expected 4, got %d", engine.minSuccessfulToolCalls)
	}

	engine.RequireAnySuccessfulTools("  zqk_write_file  ", "", "  ")
	if len(engine.requiredAnySuccessfulTools) != 1 || engine.requiredAnySuccessfulTools[0] != "zqk_write_file" {
		t.Errorf("unexpected requiredAnySuccessfulTools: %v", engine.requiredAnySuccessfulTools)
	}
}

func TestSwarmEngine_ExtractBashCodeBlockToolCalls(t *testing.T) {
	t.Parallel()

	// No bash block
	if calls := extractBashCodeBlockToolCalls("plain narrative content"); calls != nil {
		t.Errorf("expected nil for plain content, got %v", calls)
	}

	// Unclosed block
	if calls := extractBashCodeBlockToolCalls("```bash\necho unclosed"); calls != nil {
		t.Errorf("expected nil for unclosed block, got %v", calls)
	}

	// Empty script
	if calls := extractBashCodeBlockToolCalls("```bash\n   \n```"); calls != nil {
		t.Errorf("expected nil for empty script, got %v", calls)
	}

	// Valid bash script
	calls := extractBashCodeBlockToolCalls("Before\n```bash\necho hello world\n```\nAfter")
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if !strings.Contains(calls[0].Arguments, "echo hello world") {
		t.Errorf("expected script in args, got %s", calls[0].Arguments)
	}

	// Valid sh script
	callsSh := extractBashCodeBlockToolCalls("```sh\nls -la\n```")
	if len(callsSh) != 1 {
		t.Fatalf("expected 1 tool call for sh, got %d", len(callsSh))
	}
	if !strings.Contains(callsSh[0].Arguments, "ls -la") {
		t.Errorf("expected script in args, got %s", callsSh[0].Arguments)
	}
}

func TestSwarmEngine_InitialGetToolsFailure(t *testing.T) {
	t.Parallel()

	failingExec := &mockElevationExecutor{
		toolsErr: errors.New("simulated initial tools failure"),
	}
	engine := NewEngine(nil, failingExec, 1024, "eng-fail-init")

	_, err := engine.Run(context.Background(), "system", "user")
	if err == nil {
		t.Fatalf("expected error from initial GetTools failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get tools initially") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSwarmEngine_StructuredCompletionFailure(t *testing.T) {
	t.Parallel()

	exec := &mockElevationExecutor{tools: []llm.ToolDefinition{{Name: "zqk_read_file"}}}
	failingClient := &mockElevationClient{
		err: errors.New("upstream LLM timeout"),
	}
	engine := NewEngine(failingClient, exec, 1024, "eng-llm-fail")

	_, err := engine.Run(context.Background(), "system", "user")
	if err == nil {
		t.Fatalf("expected error from LLM completion failure, got nil")
	}
	if !strings.Contains(err.Error(), "generation failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSwarmEngine_MaxToolSchemaSizeCompression(t *testing.T) {
	t.Parallel()

	tools := []llm.ToolDefinition{
		{
			Name:        "zqk_verbose_tool",
			Description: strings.Repeat("Extremely verbose tool documentation payload. ", 20),
			Parameters:  map[string]any{"type": "object"},
		},
	}
	exec := &mockElevationExecutor{tools: tools}
	client := &mockElevationClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "Final compressed schema outcome"},
		},
	}
	// Set maxToolSchemaSize very small to trigger description stripping
	engine := NewEngine(client, exec, 50, "eng-schema-compress")

	res, err := engine.Run(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("unexpected engine run error: %v", err)
	}
	if res != "Final compressed schema outcome" {
		t.Errorf("expected final outcome, got %q", res)
	}
}

func TestSwarmEngine_OnStepCallback(t *testing.T) {
	t.Parallel()

	exec := &mockElevationExecutor{tools: []llm.ToolDefinition{{Name: "zqk_read_file"}}}
	client := &mockElevationClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "Step-by-step progress update"},
		},
	}
	engine := NewEngine(client, exec, 1024, "eng-onstep")

	var stepUpdates []string
	engine.OnStep = func(ctx context.Context, step string) error {
		stepUpdates = append(stepUpdates, step)
		return nil
	}

	res, err := engine.Run(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if res != "Step-by-step progress update" {
		t.Errorf("unexpected result: %q", res)
	}
	if len(stepUpdates) != 1 {
		t.Fatalf("expected 1 step update, got %d", len(stepUpdates))
	}
	if !strings.Contains(stepUpdates[0], "Step 0:") {
		t.Errorf("expected Step 0 in update, got %q", stepUpdates[0])
	}
}

func TestSwarmEngine_PreToolHookAbort(t *testing.T) {
	t.Parallel()

	exec := &mockElevationExecutor{
		tools: []llm.ToolDefinition{{Name: "zqk_read_file"}},
	}
	client := &mockElevationClient{
		responses: []llm.StructuredCompletionResponse{
			{
				ToolCalls: []llm.ToolCall{
					{ID: "call_pre", Name: "zqk_read_file", Arguments: `{"path":"main.go"}`},
				},
			},
		},
	}
	engine := NewEngine(client, exec, 1024, "eng-pre-hook-abort")
	hook := &mockSwarmHook{preToolErr: errors.New("policy violation in PreTool")}
	engine.SetHooks(hook)

	_, err := engine.Run(context.Background(), "system", "user")
	if err == nil {
		t.Fatalf("expected run error from PreTool hook, got nil")
	}
	if !strings.Contains(err.Error(), "policy violation in PreTool") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSwarmEngine_PostToolHookAbort(t *testing.T) {
	t.Parallel()

	exec := &mockElevationExecutor{
		tools: []llm.ToolDefinition{{Name: "zqk_read_file"}},
	}
	client := &mockElevationClient{
		responses: []llm.StructuredCompletionResponse{
			{
				ToolCalls: []llm.ToolCall{
					{ID: "call_post", Name: "zqk_read_file", Arguments: `{"path":"main.go"}`},
				},
			},
		},
	}
	engine := NewEngine(client, exec, 1024, "eng-post-hook-abort")
	hook := &mockSwarmHook{postToolErr: errors.New("post-execution security violation")}
	engine.SetHooks(hook)

	_, err := engine.Run(context.Background(), "system", "user")
	if err == nil {
		t.Fatalf("expected run error from PostTool hook, got nil")
	}
	if !strings.Contains(err.Error(), "post-execution security violation") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSwarmEngine_PostToolSteeringPrompt(t *testing.T) {
	t.Parallel()

	exec := &mockElevationExecutor{
		tools: []llm.ToolDefinition{{Name: "zqk_read_file"}},
	}
	client := &mockElevationClient{
		responses: []llm.StructuredCompletionResponse{
			{
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "zqk_read_file", Arguments: `{"path":"main.go"}`},
				},
			},
			{Content: "Completed with steering feedback applied"},
		},
	}
	engine := NewEngine(client, exec, 1024, "eng-steer-prompt")
	hook := &mockSwarmHook{steerPrompt: "Guidance: Inspect tests next"}
	engine.SetHooks(hook)

	res, err := engine.Run(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("unexpected engine error: %v", err)
	}
	if res != "Completed with steering feedback applied" {
		t.Errorf("unexpected result: %q", res)
	}
}

func TestSwarmMCPClient_NewExecutorConstructors(t *testing.T) {
	ctx := context.Background()
	tempProjectDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), tempProjectDir)
	t.Setenv(zqkenv.TestRoot().Name(), tempProjectDir)

	// Invalid empty mcpPath
	_, err := NewMCPExecutor(ctx, "")
	if err == nil {
		t.Fatalf("expected error for empty mcpPath, got nil")
	}
	if !strings.Contains(err.Error(), "invalid mcp path") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Nonexistent binary path
	tempWorkDir := t.TempDir()
	_, err = NewMCPExecutorAt(ctx, "/path/that/definitely/does/not/exist/binary", tempWorkDir)
	if err == nil {
		t.Fatalf("expected error for nonexistent binary, got nil")
	}
}

func TestSwarmMCPClient_CompressSchema(t *testing.T) {
	t.Parallel()

	// Nil schema safely handled
	compressSchema(nil)

	schema := map[string]any{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"format":               "uri",
		"default":              "fallback",
		"examples":             []any{"ex1", "ex2"},
		"additionalProperties": false,
		"patternProperties":    map[string]any{"^x-": map[string]any{}},
		"type":                 "object",
		"properties": map[string]any{
			"nested": map[string]any{
				"format":   "date",
				"examples": []any{"2026-01-01"},
				"type":     "string",
			},
		},
		"items": map[string]any{
			"default": "item_val",
			"type":    "string",
		},
	}

	compressSchema(schema)

	// Verify stripped keys at root
	for _, forbidden := range []string{"$schema", "format", "default", "examples", "additionalProperties", "patternProperties"} {
		if _, exists := schema[forbidden]; exists {
			t.Errorf("expected %s to be stripped from root", forbidden)
		}
	}

	// Verify stripped keys in nested properties
	props := schema["properties"].(map[string]any)
	nested := props["nested"].(map[string]any)
	if _, exists := nested["format"]; exists {
		t.Errorf("expected format to be stripped from nested property")
	}
	if _, exists := nested["examples"]; exists {
		t.Errorf("expected examples to be stripped from nested property")
	}

	// Verify stripped keys in items
	items := schema["items"].(map[string]any)
	if _, exists := items["default"]; exists {
		t.Errorf("expected default to be stripped from items")
	}
}

func TestSwarmMCPClient_McpChildEnviron(t *testing.T) {
	t.Parallel()

	parent := []string{
		"PATH=/usr/bin",
		zqkenv.AgentWorktreeRoot().Name() + "=/old/worktree",
		zqkenv.Session().Name() + "=EXISTING-SES-1",
		zqkenv.SessionID().Name() + "=EXISTING-SES-1",
	}

	// Worktree override
	res := mcpChildEnviron(parent, "/custom/worktree")
	var foundWorktree, foundOldWorktree bool
	for _, e := range res {
		if e == zqkenv.AgentWorktreeRoot().Name()+"=/custom/worktree" {
			foundWorktree = true
		}
		if e == zqkenv.AgentWorktreeRoot().Name()+"=/old/worktree" {
			foundOldWorktree = true
		}
	}
	if !foundWorktree {
		t.Errorf("expected custom worktree in environ")
	}
	if foundOldWorktree {
		t.Errorf("expected old worktree to be replaced")
	}

	// Missing session auto-injection
	minimalParent := []string{"PATH=/bin"}
	injected := mcpChildEnviron(minimalParent, "")
	var hasSession, hasSessionID bool
	for _, e := range injected {
		if strings.HasPrefix(e, zqkenv.Session().Name()+"=") {
			hasSession = true
		}
		if strings.HasPrefix(e, zqkenv.SessionID().Name()+"=") {
			hasSessionID = true
		}
	}
	if !hasSession || !hasSessionID {
		t.Errorf("expected session auto-injection: session=%v, sessionID=%v", hasSession, hasSessionID)
	}
}

func TestSwarmMCPClient_ExecuteToolCallMetaTools(t *testing.T) {
	t.Parallel()

	exec := &MCPExecutor{
		lazyTools: []llm.ToolDefinition{
			{Name: "lazy_analyzer", Description: "Analyzes system architecture"},
		},
		allTools: map[string]llm.ToolDefinition{
			"lazy_analyzer": {Name: "lazy_analyzer", Description: "Analyzes system architecture"},
		},
	}

	ctx := context.Background()

	// 1. mcp_list_tools
	listCall := llm.ToolCall{Name: DefaultToolPrefix() + "mcp_list_tools"}
	listRes, err := exec.ExecuteToolCall(ctx, listCall)
	if err != nil {
		t.Fatalf("unexpected error calling mcp_list_tools: %v", err)
	}
	if !strings.Contains(listRes, "lazy_analyzer") {
		t.Errorf("expected lazy_analyzer in list output, got: %s", listRes)
	}

	// 2. mcp_get_tool_schema - invalid arguments JSON
	badSchemaCall := llm.ToolCall{
		Name:      DefaultToolPrefix() + "mcp_get_tool_schema",
		Arguments: "invalid-json",
	}
	if _, err := exec.ExecuteToolCall(ctx, badSchemaCall); err == nil {
		t.Fatalf("expected error for bad arguments JSON, got nil")
	}

	// 3. mcp_get_tool_schema - tool found
	validSchemaCall := llm.ToolCall{
		Name:      DefaultToolPrefix() + "mcp_get_tool_schema",
		Arguments: `{"tool_name": "lazy_analyzer"}`,
	}
	schemaRes, err := exec.ExecuteToolCall(ctx, validSchemaCall)
	if err != nil {
		t.Fatalf("unexpected error getting schema: %v", err)
	}
	if !strings.Contains(schemaRes, "lazy_analyzer") {
		t.Errorf("expected lazy_analyzer in schema, got: %s", schemaRes)
	}

	// 4. mcp_get_tool_schema - tool not found
	missingSchemaCall := llm.ToolCall{
		Name:      DefaultToolPrefix() + "mcp_get_tool_schema",
		Arguments: `{"tool_name": "unknown_tool"}`,
	}
	if _, err := exec.ExecuteToolCall(ctx, missingSchemaCall); err == nil {
		t.Fatalf("expected error for missing tool schema, got nil")
	}

	// 5. mcp_call_tool - invalid arguments
	badCallTool := llm.ToolCall{
		Name:      DefaultToolPrefix() + "mcp_call_tool",
		Arguments: "not-json",
	}
	if _, err := exec.ExecuteToolCall(ctx, badCallTool); err == nil {
		t.Fatalf("expected error for invalid json in mcp_call_tool, got nil")
	}

	// 6. mcp_call_tool - empty tool name
	emptyToolCall := llm.ToolCall{
		Name:      DefaultToolPrefix() + "mcp_call_tool",
		Arguments: `{"tool_name": "   "}`,
	}
	if _, err := exec.ExecuteToolCall(ctx, emptyToolCall); err == nil {
		t.Fatalf("expected error for empty tool name, got nil")
	}

	// 7. mcp_call_tool - unregistered tool
	unregToolCall := llm.ToolCall{
		Name:      DefaultToolPrefix() + "mcp_call_tool",
		Arguments: `{"tool_name": "unregistered_tool"}`,
	}
	if _, err := exec.ExecuteToolCall(ctx, unregToolCall); err == nil {
		t.Fatalf("expected error for unregistered lazy tool, got nil")
	}
}

func TestSwarmMCPClient_NormalizeToolNameAndTelemetry(t *testing.T) {
	t.Parallel()

	exec := &MCPExecutor{
		tools: []llm.ToolDefinition{
			{Name: "zqk_read_file"},
			{Name: "zqk_write_file"},
		},
		allTools: map[string]llm.ToolDefinition{
			"zqk_read_file":  {Name: "zqk_read_file"},
			"zqk_write_file": {Name: "zqk_write_file"},
		},
	}

	// Space replacement normalization
	norm, err := exec.normalizeToolName("zqk read file")
	if err != nil || norm != "zqk_read_file" {
		t.Errorf("expected zqk_read_file from space replacement, got %q, err: %v", norm, err)
	}

	// Fuzzy match within threshold <= 3
	fuzzy, err := exec.normalizeToolName("zqk_read_fil")
	if err != nil || fuzzy != "zqk_read_file" {
		t.Errorf("expected zqk_read_file from fuzzy match, got %q, err: %v", fuzzy, err)
	}

	// Missing tool with distance > 3
	_, err = exec.normalizeToolName("completely_different_unknown_tool")
	if err == nil {
		t.Fatalf("expected error for unknown tool, got nil")
	}

	// SetTelemetryContext and Close
	tracker := telemetry.NewTracker(nil)
	exec.SetTelemetryContext("persona-c218", tracker)
	if exec.personaID != "persona-c218" {
		t.Errorf("expected personaID to be set, got %q", exec.personaID)
	}

	// Close safely with nil subprocess / files
	if err := exec.Close(); err != nil {
		t.Errorf("unexpected error closing executor: %v", err)
	}
}

func TestSwarmMCPClient_ExecuteToolCallWithMockClient(t *testing.T) {
	t.Parallel()

	resultObj := mcp.ToolCallResult{
		Content: []mcp.Content{
			{Type: "text", Text: `{"status":"ok"}`},
		},
		IsError: false,
	}
	resultBytes, err := json.Marshal(resultObj)
	if err != nil {
		t.Fatalf("failed to marshal resultObj: %v", err)
	}

	mockResp := mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(resultBytes),
	}
	respBytes, err := json.Marshal(mockResp)
	if err != nil {
		t.Fatalf("failed to marshal mockResp: %v", err)
	}

	transport := &mockMCPTransport{readResp: respBytes}
	in := bytes.NewBuffer(nil)
	out := bytes.NewBuffer(nil)

	client := mcp.NewClient(in, out, transport)
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Errorf("failed to close client: %v", closeErr)
		}
	}()

	exec := &MCPExecutor{
		client: client,
		tools: []llm.ToolDefinition{
			{Name: "zqk_echo"},
		},
		allTools: map[string]llm.ToolDefinition{
			"zqk_echo": {Name: "zqk_echo"},
		},
	}

	tracker := telemetry.NewTracker(nil)
	exec.SetTelemetryContext("test-persona", tracker)

	call := llm.ToolCall{
		Name:      "zqk_echo",
		Arguments: `{"msg":"hello"}`,
	}
	res, err := exec.ExecuteToolCall(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error executing tool call: %v", err)
	}
	if res != `{"status":"ok"}` {
		t.Errorf("expected '{\"status\":\"ok\"}', got %q", res)
	}

	// Invalid arguments JSON
	badCall := llm.ToolCall{
		Name:      "zqk_echo",
		Arguments: "{broken-json",
	}
	if _, err := exec.ExecuteToolCall(context.Background(), badCall); err == nil {
		t.Fatalf("expected error for broken tool arguments json, got nil")
	}
}

func TestSwarmMCPClient_GetToolsWithMockClient(t *testing.T) {
	t.Parallel()

	toolsList := mcp.ToolsListResult{
		Tools: []mcp.Tool{
			{
				Name:        "zqk_read_file",
				Description: "Read a file from disk",
				InputSchema: map[string]any{"type": "object"},
			},
			{
				Name:        "deep_architectural_analyzer",
				Description: "Performs complex AST graph analysis",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}
	resultBytes, err := json.Marshal(toolsList)
	if err != nil {
		t.Fatalf("failed to marshal tools list: %v", err)
	}

	mockResp := mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(resultBytes),
	}
	respBytes, err := json.Marshal(mockResp)
	if err != nil {
		t.Fatalf("failed to marshal mockResp: %v", err)
	}

	transport := &mockMCPTransport{readResp: respBytes}
	in := bytes.NewBuffer(nil)
	out := bytes.NewBuffer(nil)

	client := mcp.NewClient(in, out, transport)
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Errorf("failed to close client: %v", closeErr)
		}
	}()

	exec := &MCPExecutor{client: client}

	ctx := context.Background()
	tools, err := exec.GetTools(ctx)
	if err != nil {
		t.Fatalf("unexpected error from GetTools: %v", err)
	}

	if len(tools) == 0 {
		t.Fatalf("expected tools to be populated, got 0")
	}

	// Verify eager tool partition and meta-tools injection
	var hasReadFile, hasListTools, hasCallTool bool
	for _, tool := range tools {
		if tool.Name == "zqk_read_file" {
			hasReadFile = true
		}
		if tool.Name == DefaultToolPrefix()+"mcp_list_tools" {
			hasListTools = true
		}
		if tool.Name == DefaultToolPrefix()+"mcp_call_tool" {
			hasCallTool = true
		}
	}
	if !hasReadFile || !hasListTools || !hasCallTool {
		t.Errorf("expected eager tool and meta-tools: readFile=%v, listTools=%v, callTool=%v",
			hasReadFile, hasListTools, hasCallTool)
	}

	// Verify lazy tools partition
	if len(exec.lazyTools) != 1 || exec.lazyTools[0].Name != "deep_architectural_analyzer" {
		t.Errorf("expected deep_architectural_analyzer in lazyTools, got %v", exec.lazyTools)
	}

	// Calling GetTools again returns cached slice
	cachedTools, err := exec.GetTools(ctx)
	if err != nil || len(cachedTools) != len(tools) {
		t.Errorf("expected cached tools to be returned without error")
	}
}

func TestSwarmLLMTrace_FormattingAndSanitization(t *testing.T) {
	t.Parallel()

	// Sanitize trace ID
	sanitized := sanitizeTraceID("  session/123:abc#test*  ")
	if sanitized != "session_123_abc_test_" {
		t.Errorf("unexpected sanitized ID: %q", sanitized)
	}
	if sanitizeTraceID("") != "unknown" {
		t.Errorf("expected 'unknown' for empty ID")
	}

	// Format trace with full fields
	rec := llmTraceRecord{
		EngineID:           "test-eng",
		Step:               2,
		ToolChoice:         "auto",
		ToolChoiceFunction: "zqk_read_file",
		ToolNames:          []string{"zqk_read_file", "zqk_write_file"},
		Prompt: []llm.Message{
			{
				Role:       "tool",
				Name:       "zqk_read_file",
				ToolCallID: "call_abc",
				Content:    "package main\n",
			},
			{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{
					{ID: "call_next", Name: "zqk_write_file", Arguments: `{"path":"out.txt"}`},
				},
			},
		},
		RawContent: "I have edited the file.",
		NativeToolCalls: []llm.ToolCall{
			{ID: "call_native", Name: "zqk_write_file", Arguments: "{}"},
		},
		DroppedToolNames: []string{"zqk_lookup"},
	}

	formatted := formatLLMTrace(rec)
	if !strings.Contains(formatted, "LLM STEP 2") {
		t.Errorf("expected step number in formatted trace")
	}
	if !strings.Contains(formatted, "pin_filter_dropped: zqk_lookup") {
		t.Errorf("expected dropped tools in trace")
	}
	if !strings.Contains(formatted, "I have edited the file.") {
		t.Errorf("expected raw content in trace")
	}
}

func TestSwarmLLMTrace_WriteTraceToDisk(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(zqkenv.LLMTrace().Name(), "1")
	t.Setenv(zqkenv.LLMTraceDir().Name(), tempDir)

	if !llmTraceEnabled() {
		t.Errorf("expected llmTraceEnabled() to be true when LLM_TRACE=1")
	}
	if dir := llmTraceDir(); dir != tempDir {
		t.Errorf("expected llmTraceDir() to equal %q, got %q", tempDir, dir)
	}

	rec := llmTraceRecord{
		EngineID:   "session-test-disk",
		Step:       0,
		RawContent: "Traced LLM response content",
	}
	writeLLMTrace(rec)

	expectedFile := filepath.Join(tempDir, "llm-trace-session-test-disk.log")
	content, err := fileutil.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("failed to read written LLM trace file: %v", err)
	}
	if !strings.Contains(string(content), "Traced LLM response content") {
		t.Errorf("expected content in trace file, got: %s", string(content))
	}
}

func TestSwarmHooks_LifecycleAndCatastrophicErrorGuard(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 1. ToolDenialGuard
	tdGuard := NewToolDenialGuard()
	if err := tdGuard.PreTool(ctx, llm.ToolCall{Name: "zqk_read_file"}); err != nil {
		t.Errorf("unexpected error in PreTool: %v", err)
	}
	tdGuard.OnSuccess(ctx, llm.ToolCall{Name: "zqk_read_file"}, "ok")

	// 2. HallucinationCircuitBreaker
	hcb := NewHallucinationCircuitBreaker()
	if err := hcb.PreTool(ctx, llm.ToolCall{Name: "zqk_read_file"}); err != nil {
		t.Errorf("unexpected error in PreTool: %v", err)
	}
	hcb.OnSuccess(ctx, llm.ToolCall{Name: "zqk_read_file"}, "ok")

	// Three consecutive file-not-found errors triggers guidance
	for i := 1; i <= 2; i++ {
		steer, err := hcb.PostTool(ctx, llm.ToolCall{Name: "zqk_read_file"}, "no such file or directory", nil)
		if err != nil || steer != "" {
			t.Errorf("expected no steer/err at step %d, got steer=%q err=%v", i, steer, err)
		}
	}
	steer, err := hcb.PostTool(ctx, llm.ToolCall{Name: "zqk_read_file"}, "no such file or directory", nil)
	if err != nil || !strings.Contains(steer, "Stop guessing file paths") {
		t.Errorf("expected guidance at step 3, got steer=%q err=%v", steer, err)
	}
	// 4th error triggers abort
	_, err = hcb.PostTool(ctx, llm.ToolCall{Name: "zqk_read_file"}, "no such file or directory", nil)
	if err == nil || !strings.Contains(err.Error(), "4 consecutive file-not-found errors") {
		t.Errorf("expected abort error at step 4, got: %v", err)
	}

	// 3. ProactiveWorkspaceSeeder
	seeder := &ProactiveWorkspaceSeeder{}
	if err := seeder.PreTool(ctx, llm.ToolCall{Name: "write_code"}); err != nil {
		t.Errorf("unexpected error in PreTool: %v", err)
	}
	if _, err := seeder.PostTool(ctx, llm.ToolCall{Name: "write_code"}, "ok", nil); err != nil {
		t.Errorf("unexpected error in PostTool: %v", err)
	}
	seeder.OnSuccess(ctx, llm.ToolCall{Name: "write_code"}, "ok")
	seeder.OnSuccess(ctx, llm.ToolCall{Name: "read_file"}, "ok")

	// 4. CatastrophicErrorGuard
	ceGuard := NewCatastrophicErrorGuard()
	if err := ceGuard.PreTool(ctx, llm.ToolCall{Name: "zqk_read_file"}); err != nil {
		t.Errorf("unexpected error in PreTool: %v", err)
	}

	// Immediate fatal auth failure
	_, err = ceGuard.PostTool(ctx, llm.ToolCall{Name: "zqk_read_file"}, "POL-AGENT-ACCOUNT-LOGIN-001 account revoked", nil)
	if err == nil || !strings.Contains(err.Error(), "unrecoverable auth/identity failure") {
		t.Errorf("expected immediate catastrophic failure, got: %v", err)
	}

	// Clean success resets counter
	ceGuard.OnSuccess(ctx, llm.ToolCall{Name: "zqk_read_file"}, "ok")
}

func TestSwarmToolGuard_ObjectListLimitAndKinds(t *testing.T) {
	t.Parallel()

	// objectListLimit type variants
	if n, ok := objectListLimit(nil); ok || n != 0 {
		t.Errorf("expected (0, false) for nil args")
	}
	if n, ok := objectListLimit(map[string]any{"other": 10}); ok || n != 0 {
		t.Errorf("expected (0, false) when limit key missing")
	}
	if n, ok := objectListLimit(map[string]any{"limit": float64(15)}); !ok || n != 15 {
		t.Errorf("expected (15, true) for float64, got (%d, %v)", n, ok)
	}
	if n, ok := objectListLimit(map[string]any{"limit": 12}); !ok || n != 12 {
		t.Errorf("expected (12, true) for int, got (%d, %v)", n, ok)
	}
	if n, ok := objectListLimit(map[string]any{"limit": json.Number("8")}); !ok || n != 8 {
		t.Errorf("expected (8, true) for json.Number, got (%d, %v)", n, ok)
	}
	if n, ok := objectListLimit(map[string]any{"limit": " 18 "}); !ok || n != 18 {
		t.Errorf("expected (18, true) for string number, got (%d, %v)", n, ok)
	}
	if _, ok := objectListLimit(map[string]any{"limit": "not-a-number"}); ok {
		t.Errorf("expected false for invalid string limit")
	}
	if _, ok := objectListLimit(map[string]any{"limit": true}); ok {
		t.Errorf("expected false for boolean limit")
	}

	// oversizedObjectList boundary tests
	if oversizedObjectList(`{"limit": 25}`) != true {
		t.Errorf("expected limit 25 to be oversized")
	}
	if oversizedObjectList(`{"limit": 0}`) != true {
		t.Errorf("expected limit 0 to be oversized")
	}
	if oversizedObjectList(`{"limit": 10}`) != false {
		t.Errorf("expected limit 10 to be valid")
	}

	// extractedObjectKind variants
	if k := extractedObjectKind("invalid-json"); k != "" {
		t.Errorf("expected empty string for invalid json, got %q", k)
	}
	if k := extractedObjectKind(`{"kind": "goal"}`); k != "goal" {
		t.Errorf("expected 'goal', got %q", k)
	}
	if k := extractedObjectKind(`{"object_kind": "milestone"}`); k != "milestone" {
		t.Errorf("expected 'milestone', got %q", k)
	}
	if isInventedObjectKind(`{"kind": "eval_finding"}`) != true {
		t.Errorf("expected invented object kind for 'eval_finding'")
	}
	if isInventedObjectKind(`{"kind": "requirement"}`) != false {
		t.Errorf("expected requirement to be recognized kind, not invented")
	}

	// placeholderObjectID variants
	if placeholderObjectID("not-json") != false {
		t.Errorf("expected false for non-json")
	}
	if placeholderObjectID(`{"id": "<id>"}`) != true {
		t.Errorf("expected placeholder with angle brackets to be true")
	}
	if placeholderObjectID(`{"object_id": "OBJECT_ID"}`) != true {
		t.Errorf("expected placeholder string literal to be true")
	}
	if placeholderObjectID(`{"id": "GOAL-12345"}`) != false {
		t.Errorf("expected real ID to be false")
	}
}

func TestSwarmTaskQueue_EdgeCasesAndConcurrency(t *testing.T) {
	t.Parallel()

	q := NewInMemoryTaskQueue()
	defer q.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled context

	// Enqueue with canceled context
	err := q.Enqueue(ctx, Task{ID: "task-canceled"})
	if err == nil {
		t.Fatalf("expected error for canceled context, got nil")
	}

	// Dequeue with canceled context
	_, err = q.Dequeue(ctx)
	if err == nil {
		t.Fatalf("expected error for canceled context, got nil")
	}

	// Concurrent enqueue and dequeue
	activeQ := NewInMemoryTaskQueue()
	var wg sync.WaitGroup
	numTasks := 20

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		taskID := fmt.Sprintf("task-%d", i)
		goroutinelabels.NewGoroutine("test", "swarm_enqueue").StartSimple(func() {
			defer wg.Done()
			if enqErr := activeQ.Enqueue(context.Background(), Task{ID: taskID}); enqErr != nil {
				t.Errorf("failed to enqueue: %v", enqErr)
			}
		})
	}
	wg.Wait()

	if activeQ.Size() != numTasks {
		t.Errorf("expected size %d, got %d", numTasks, activeQ.Size())
	}

	// Dequeue all tasks
	for i := 0; i < numTasks; i++ {
		task, dErr := activeQ.Dequeue(context.Background())
		if dErr != nil || task.ID == "" {
			t.Fatalf("failed to dequeue task %d: %v", i, dErr)
		}
	}

	// Close queue and verify idempotent close
	activeQ.Close()
	activeQ.Close()

	// Enqueue on closed queue
	if err := activeQ.Enqueue(context.Background(), Task{ID: "task-after-close"}); err == nil {
		t.Errorf("expected error enqueuing to closed queue")
	}

	// Dequeue on closed empty queue
	if _, err := activeQ.Dequeue(context.Background()); err != io.EOF {
		t.Errorf("expected io.EOF on closed empty queue, got %v", err)
	}
}

func TestSwarmDispatcherAndFeedback_EdgeCases(t *testing.T) {
	t.Parallel()

	// 1. runnerHandler non-task payload
	runner := &mockSwarmRunner{}
	handler := &runnerHandler{runner: runner}

	payloadRes, err := handler.Handle(context.Background(), "not-a-task-struct", 0)
	if err != nil || payloadRes != nil {
		t.Errorf("expected (nil, nil) for non-task payload, got res=%v, err=%v", payloadRes, err)
	}

	// Valid task payload
	taskPayload := Task{ID: "task-handle", SystemPrompt: "sys", UserPrompt: "usr"}
	taskRes, err := handler.Handle(context.Background(), taskPayload, 0)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	tr, ok := taskRes.(TaskResult)
	if !ok || tr.Task.ID != "task-handle" {
		t.Errorf("unexpected task result: %v", taskRes)
	}

	// 2. FeedbackProcessor - context.Canceled passes through
	q := NewInMemoryTaskQueue()
	defer q.Close()
	fp := NewFeedbackProcessor(q)

	inChan := make(chan TaskResult, 5)
	outChan := fp.Process(context.Background(), inChan)

	// Send canceled result
	inChan <- TaskResult{
		Task:  Task{ID: "task-cancel-pass"},
		Error: context.Canceled,
	}

	// Send retriable failed result
	inChan <- TaskResult{
		Task:  Task{ID: "task-retry", CurrentRetries: 0, MaxRetries: 2, UserPrompt: "initial"},
		Error: errors.New("temporary execution flake"),
	}

	// Send max retries reached result
	inChan <- TaskResult{
		Task:  Task{ID: "task-exhausted", CurrentRetries: 2, MaxRetries: 2},
		Error: errors.New("persistent failure"),
	}

	close(inChan)

	// Verify canceled result received on outChan
	res1 := <-outChan
	if res1.Task.ID != "task-cancel-pass" || !errors.Is(res1.Error, context.Canceled) {
		t.Errorf("expected canceled task to pass through, got %v", res1)
	}

	// Verify exhausted result received on outChan
	res2 := <-outChan
	if res2.Task.ID != "task-exhausted" {
		t.Errorf("expected exhausted task to pass through, got %v", res2)
	}

	// Verify retried task was enqueued back to q
	retriedTask, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("expected retried task in queue, got error: %v", err)
	}
	if retriedTask.ID != "task-retry" || retriedTask.CurrentRetries != 1 {
		t.Errorf("unexpected retried task: %v", retriedTask)
	}
	if !strings.Contains(retriedTask.UserPrompt, "[System Feedback:") {
		t.Errorf("expected system feedback appended to user prompt, got: %s", retriedTask.UserPrompt)
	}
}

func TestSwarmPrompts_TemplateResolutionAndStorage(t *testing.T) {
	t.Parallel()

	// 1. ResolvePromptTemplate fallbacks
	fallback := "Default fallback template body: {{.WorkerID}}"
	if res := ResolvePromptTemplate(nil, nil, nil, "any", fallback); res != fallback {
		t.Errorf("expected fallback for nil storage provider, got %q", res)
	}
	if res := ResolvePromptTemplate(context.Background(), &mockPromptStorageProvider{}, nil, "", fallback); res != fallback {
		t.Errorf("expected fallback for empty templateID, got %q", res)
	}

	// Storage read returns error -> fallback
	errProvider := &mockPromptStorageProvider{
		readFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, errors.New("storage failure")
		},
	}
	if res := ResolvePromptTemplate(context.Background(), errProvider, nil, "tmpl-id", fallback); res != fallback {
		t.Errorf("expected fallback on read error, got %q", res)
	}

	// Storage read returns object with valid prompt body
	storedProvider := &mockPromptStorageProvider{
		readFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			if id == PromptTemplateSwarmWorkerTask {
				return map[string]any{
					objects.FieldKeyPromptBody: "Custom stored prompt for {{.TaskName}}",
				}, nil
			}
			return map[string]any{
				objects.FieldKeyPromptBody: "Custom stored prompt for {{.WorkerID}}",
			}, nil
		},
	}
	stored := ResolvePromptTemplate(context.Background(), storedProvider, nil, "tmpl-id", fallback)
	if stored != "Custom stored prompt for {{.WorkerID}}" {
		t.Errorf("expected stored body, got %q", stored)
	}

	// 2. renderPromptTemplate fallback on syntax error in custom template
	rendered, err := renderPromptTemplate("test_tmpl", "Invalid template {{if}}", "Fallback {{.Name}}", map[string]string{"Name": "ZQK"})
	if err != nil {
		t.Fatalf("unexpected error with fallback: %v", err)
	}
	if rendered != "Fallback ZQK" {
		t.Errorf("expected 'Fallback ZQK', got %q", rendered)
	}

	// 3. RenderSystemPromptWithStorage & RenderTaskPromptWithStorage
	sysData := SwarmWorkerSystemData{
		WorkerID:  "worker-c218",
		WorkClass: "docs_eval",
	}
	sysPrompt, err := RenderSystemPromptWithStorage(context.Background(), storedProvider, nil, sysData)
	if err != nil {
		t.Fatalf("unexpected system prompt render error: %v", err)
	}
	if !strings.Contains(sysPrompt, "Custom stored prompt for worker-c218") {
		t.Errorf("expected custom stored template to be rendered, got: %s", sysPrompt)
	}

	taskData := SwarmWorkerTaskData{
		TaskName: "worker-task-c218",
	}
	taskPrompt, err := RenderTaskPromptWithStorage(context.Background(), storedProvider, nil, taskData)
	if err != nil {
		t.Fatalf("unexpected task prompt render error: %v", err)
	}
	if !strings.Contains(taskPrompt, "Custom stored prompt for") {
		t.Errorf("expected custom task template to be rendered, got: %s", taskPrompt)
	}
}

func TestSwarmTokenTracker_CompactHistoryEdgeCases(t *testing.T) {
	t.Parallel()

	tracker := NewTokenTracker(100, 0.9)
	tools := []llm.ToolDefinition{{Name: "tool1"}}

	// <= 2 messages
	messages2 := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "usr"},
	}
	compacted2, ok2 := tracker.CompactHistory(messages2, tools)
	if ok2 {
		t.Errorf("expected ok=false for 2 messages")
	}
	if len(compacted2) != 2 {
		t.Errorf("expected 2 messages preserved")
	}

	// 3 messages (odd boundary)
	messages3 := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "usr"},
		{Role: "assistant", Content: strings.Repeat("huge assistant message ", 50)},
	}
	compacted3, ok3 := tracker.CompactHistory(messages3, tools)
	if !ok3 || len(compacted3) != 2 {
		t.Errorf("expected compaction to reduce to 2 messages, ok=%v len=%d", ok3, len(compacted3))
	}

	// Unachievable budget even with 2 messages
	tinyTracker := NewTokenTracker(5, 0.9)
	hugeMessages := []llm.Message{
		{Role: "system", Content: strings.Repeat("massive system prompt ", 20)},
		{Role: "user", Content: "user query"},
		{Role: "assistant", Content: "resp"},
		{Role: "user", Content: "next"},
	}
	_, okHuge := tinyTracker.CompactHistory(hugeMessages, tools)
	if okHuge {
		t.Errorf("expected ok=false when budget unachievable")
	}
}

func TestSwarmMCPClient_ExecuteCallToolSuccessAndCloseBranches(t *testing.T) {
	t.Parallel()

	resultObj := mcp.ToolCallResult{
		Content: []mcp.Content{
			{Type: "text", Text: `{"result":"meta_success"}`},
		},
		IsError: false,
	}
	resultBytes, err := json.Marshal(resultObj)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	mockResp := mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(resultBytes),
	}
	respBytes, err := json.Marshal(mockResp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	transport := &mockMCPTransport{readResp: respBytes}
	in := bytes.NewBuffer(nil)
	out := bytes.NewBuffer(nil)
	client := mcp.NewClient(in, out, transport)

	tempFile, err := fileutil.CreateTemp(t.TempDir(), "stderr-*.log")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	canceled := false
	cancelFunc := func() {
		canceled = true
	}

	exec := &MCPExecutor{
		client:     client,
		cancel:     cancelFunc,
		stderrFile: tempFile,
		lazyTools: []llm.ToolDefinition{
			{Name: "lazy_target", Description: "target tool"},
		},
		allTools: map[string]llm.ToolDefinition{
			"lazy_target": {Name: "lazy_target"},
		},
	}

	// 1. Successful mcp_call_tool invocation
	call := llm.ToolCall{
		Name:      DefaultToolPrefix() + "mcp_call_tool",
		Arguments: `{"tool_name": "lazy_target", "arguments": {"param": "val"}}`,
	}
	res, err := exec.ExecuteToolCall(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error executing mcp_call_tool: %v", err)
	}
	if res != `{"result":"meta_success"}` {
		t.Errorf("unexpected call_tool result: %s", res)
	}

	// 2. Full Close exercising all fields (client, cancel, stderrFile)
	if err := exec.Close(); err != nil {
		t.Errorf("unexpected error on Close(): %v", err)
	}
	if !canceled {
		t.Errorf("expected cancelFunc to be called")
	}
}

func TestSwarmLLMTrace_DirectoryFallbackBranches(t *testing.T) {
	tempProjectRoot := t.TempDir()

	// When LLM_TRACE_DIR is empty but PROJECT_ROOT is set
	t.Setenv(zqkenv.LLMTraceDir().Name(), "")
	t.Setenv(zqkenv.ProjectRoot().Name(), tempProjectRoot)

	dir := llmTraceDir()
	expectedSubdir := filepath.Join(tempProjectRoot, ".zqk", "logs", "llm")
	if dir != expectedSubdir {
		t.Errorf("expected %q, got %q", expectedSubdir, dir)
	}

	// When both are empty, fallback to Getwd
	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	dirWd := llmTraceDir()
	if !strings.Contains(dirWd, ".zqk") {
		t.Errorf("expected cwd fallback to contain .zqk, got %q", dirWd)
	}
}

func TestSwarmEngine_StepGetToolsFailureAndCircuitBreaker(t *testing.T) {
	t.Parallel()

	// 1. Repeating pattern circuit breaker in Run loop
	repeatingClient := &mockElevationClient{
		responses: []llm.StructuredCompletionResponse{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "zqk_read_file", Arguments: `{"path":"file.go"}`}}},
			{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "zqk_read_file", Arguments: `{"path":"file.go"}`}}},
			{ToolCalls: []llm.ToolCall{{ID: "c3", Name: "zqk_read_file", Arguments: `{"path":"file.go"}`}}},
		},
	}
	engineCircuit := NewEngine(repeatingClient, &mockElevationExecutor{
		tools: []llm.ToolDefinition{{Name: "zqk_read_file"}},
	}, 1024, "eng-cb")

	_, err := engineCircuit.Run(context.Background(), "sys", "usr")
	if err == nil {
		t.Fatalf("expected circuit breaker error, got nil")
	}
	if !strings.Contains(err.Error(), "circuit breaker tripped") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSwarmEngine_StepLevelGetToolsFailure(t *testing.T) {
	t.Parallel()

	callCount := 0
	stepFailingExec := &mockElevationExecutor{
		getToolsFunc: func(ctx context.Context) ([]llm.ToolDefinition, error) {
			callCount++
			if callCount == 1 {
				return []llm.ToolDefinition{{Name: "zqk_read_file"}}, nil
			}
			return nil, errors.New("simulated step-level tools error")
		},
	}
	client := &mockElevationClient{
		responses: []llm.StructuredCompletionResponse{
			{Content: "should not be reached"},
		},
	}
	engine := NewEngine(client, stepFailingExec, 1024, "eng-step-fail")

	_, err := engine.Run(context.Background(), "sys", "usr")
	if err == nil {
		t.Fatalf("expected step tools error, got nil")
	}
	if !strings.Contains(err.Error(), "simulated step-level tools error") {
		t.Errorf("unexpected error message: %v", err)
	}
}
