package mcp

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// 1. handleToolCallWithContext built-in dispatch and custom handlers
func TestDeep11_ServerToolExecution_HandleToolCall(t *testing.T) {
	s := NewServer()
	tmpDir := t.TempDir()
	s.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: tmpDir}
	ctx := context.Background()

	// EventEmitter
	s.eventEmitter = NewEventEmitter(10)

	// Built-in tools:
	// 1. echo
	_, _ = s.handleToolCallWithContext(ctx, "echo", map[string]any{"message": "hi"})
	_, _ = s.handleToolCallWithContext(ctx, "test_echo", map[string]any{"message": "hi"})

	// 2. execute_bash
	_, _ = s.handleToolCallWithContext(ctx, "execute_bash", map[string]any{
		objects.FieldKeyCommand: "echo 'mcp test'",
	})

	// 3. write_file & read_file
	_, _ = s.handleToolCallWithContext(ctx, "write_file", map[string]any{
		"path":    "test.txt",
		"content": "test file content",
	})
	_, _ = s.handleToolCallWithContext(ctx, "read_file", map[string]any{
		"path": "test.txt",
	})

	// 4. write_code & read_code
	_, _ = s.handleToolCallWithContext(ctx, "write_code", map[string]any{
		"path":    "sample.go",
		"content": "package main\n\nfunc main() {}\n",
	})
	_, _ = s.handleToolCallWithContext(ctx, "read_code", map[string]any{
		"path": "sample.go",
	})

	// 5. write_code with unparseable Go code (triggers AST audit warning)
	_, _ = s.handleToolCallWithContext(ctx, "write_code", map[string]any{
		"path":    "unparseable.go",
		"content": "package main\n\nfunc broken() {",
	})

	// 6. object tools
	_, _ = s.handleToolCallWithContext(ctx, "object_get", map[string]any{"id": "BLI-1"})
	_, _ = s.handleToolCallWithContext(ctx, "object_list", map[string]any{"kind": "backlog_item"})
	_, _ = s.handleToolCallWithContext(ctx, "object_count", map[string]any{"kind": "backlog_item"})

	// 7. system tools
	_, _ = s.handleToolCallWithContext(ctx, "system_status", map[string]any{})
	_, _ = s.handleToolCallWithContext(ctx, "system_check", map[string]any{"id": "CHK-1"})

	// 8. workflow tools
	_, _ = s.handleToolCallWithContext(ctx, "get_priority_plan_items", map[string]any{"priority_plan_id": "PRI-1"})
	_, _ = s.handleToolCallWithContext(ctx, "get_current_backlog_item", map[string]any{})
	_, _ = s.handleToolCallWithContext(ctx, "get_next_backlog_item", map[string]any{})

	// 9. custom registered tool
	s.RegisterTool("my_custom_tool", "Custom tool", nil, func(ctx context.Context, args map[string]any) (any, error) {
		return "custom_res", nil
	})
	resCustom, errCustom := s.handleToolCallWithContext(ctx, "my_custom_tool", map[string]any{})
	if errCustom != nil || resCustom != "custom_res" {
		t.Errorf("expected custom tool result: %v, %v", resCustom, errCustom)
	}

	// 10. unknown tool
	_, errUnknown := s.handleToolCallWithContext(ctx, "nonexistent_custom_tool", map[string]any{})
	if errUnknown == nil {
		t.Error("expected error for nonexistent tool")
	}
}
