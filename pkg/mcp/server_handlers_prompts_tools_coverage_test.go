package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// 1. handlePromptsGet all prompt cases
func TestDeep9_ServerHandlersList_AllPrompts(t *testing.T) {
	ctx := context.Background()

	promptNames := []string{
		"welcome",
		"execution_context",
		"big_picture",
		"my_role",
		"current_role",
		"getting_started",
		"query_help",
		"create_object_template",
		"common_tasks",
		"object_lifecycle",
		"filter_syntax",
		"role_based_access",
	}

	// 1. Without generator (fallback static branches)
	s := NewServer()
	for _, name := range promptNames {
		s.RegisterPrompt(name, "desc", nil)
	}

	for _, name := range promptNames {
		params, _ := json.Marshal(map[string]any{"name": name})
		res, err := s.handlePromptsGet(ctx, "prompts/get", params)
		if err != nil || res == nil {
			t.Errorf("handlePromptsGet failed for %s: %v", name, err)
		}
	}

	// 2. With generator (dynamic branches)
	tmpDir := t.TempDir()
	sGen := NewServer()
	sGen.SetProjectRoot(tmpDir)
	sGen.SetSecurityContext(pkgctx.NewSecurityContext("ACC-DEV", []string{"developer"}, []string{"read:*"}))
	for _, name := range promptNames {
		sGen.RegisterPrompt(name, "desc", nil)
	}

	for _, name := range promptNames {
		params, _ := json.Marshal(map[string]any{
			"name": name,
			"arguments": map[string]any{
				"agent_task_id": "TASK-1",
			},
		})
		res, err := sGen.handlePromptsGet(ctx, "prompts/get", params)
		if err != nil || res == nil {
			t.Errorf("handlePromptsGet with generator failed for %s: %v", name, err)
		}
	}

	// 3. Error cases
	_, err := s.handlePromptsGet(ctx, "prompts/get", []byte(`{invalid`))
	if err == nil {
		t.Error("expected invalid params error")
	}

	paramsNotFound, _ := json.Marshal(map[string]any{"name": "nonexistent_prompt"})
	_, err = s.handlePromptsGet(ctx, "prompts/get", paramsNotFound)
	if err == nil {
		t.Error("expected not found error")
	}
}

// 2. handleToolsCall comprehensive execution branches
func TestDeep9_ServerHandlersTools_ToolsCall(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	// 1. Not initialized
	_, err := s.handleToolsCall(ctx, "tools/call", []byte(`{"name":"ping"}`))
	if err == nil {
		t.Error("expected not initialized error")
	}

	// 2. Initialized
	s.initialized.Store(true)

	// Invalid params
	_, err = s.handleToolsCall(ctx, "tools/call", []byte(`{invalid`))
	if err == nil {
		t.Error("expected parse error")
	}

	// Register a working tool
	s.RegisterTool("test_echo", "echoes args", map[string]any{}, func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{"echo": args, "format": "json"}, nil
	})

	paramsEcho, _ := json.Marshal(map[string]any{
		"name":      "test_echo",
		"arguments": map[string]any{"msg": "hi"},
	})
	res, err := s.handleToolsCall(ctx, "tools/call", paramsEcho)
	if err != nil || res == nil {
		t.Errorf("test_echo failed: %v", err)
	}

	// Register a tool that returns execution error with details map
	s.RegisterTool("test_err_map", "fails with map", map[string]any{}, func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{
			"execution_error": "cmd failed",
			"stderr":          "error on stderr",
			"stdout":          "partial out",
			"parse_error":     "json parse failed",
			"format":          "text",
		}, errors.New("command execution failed")
	})

	paramsErrMap, _ := json.Marshal(map[string]any{
		"name": "test_err_map",
	})
	resErrMap, err := s.handleToolsCall(ctx, "tools/call", paramsErrMap)
	if err != nil {
		t.Errorf("expected ToolCallResult with IsError=true, got err: %v", err)
	}
	if tcRes, ok := resErrMap.(ToolCallResult); !ok || !tcRes.IsError {
		t.Errorf("expected IsError true, got: %v", resErrMap)
	}

	// Register a tool that returns simple error
	s.RegisterTool("test_err_simple", "fails simply", map[string]any{}, func(ctx context.Context, args map[string]any) (any, error) {
		return "not a map", errors.New("simple failure")
	})
	paramsErrSimple, _ := json.Marshal(map[string]any{
		"name": "test_err_simple",
	})
	resSimple, err := s.handleToolsCall(ctx, "tools/call", paramsErrSimple)
	if err != nil {
		t.Errorf("expected ToolCallResult, got err: %v", err)
	}
	if tcRes, ok := resSimple.(ToolCallResult); !ok || !tcRes.IsError {
		t.Errorf("expected IsError true, got: %v", resSimple)
	}

	// Authorization error
	s.RegisterTool("test_auth_err", "auth error", map[string]any{}, func(ctx context.Context, args map[string]any) (any, error) {
		return nil, errors.New("Command not allowed: insufficient permissions")
	})
	paramsAuthErr, _ := json.Marshal(map[string]any{"name": "test_auth_err"})
	_, _ = s.handleToolsCall(ctx, "tools/call", paramsAuthErr)

	// Elicitation error
	s.RegisterTool("test_elicitation_err", "elicitation error", map[string]any{}, func(ctx context.Context, args map[string]any) (any, error) {
		return nil, &ElicitationError{Message: "auth required"}
	})
	paramsElicit, _ := json.Marshal(map[string]any{"name": "test_elicitation_err"})
	_, err = s.handleToolsCall(ctx, "tools/call", paramsElicit)
	if err == nil {
		t.Error("expected elicitation error returned directly")
	}
}
