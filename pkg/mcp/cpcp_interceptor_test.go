package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestCPCPInterceptor_NilParams(t *testing.T) {
	t.Parallel()

	interceptor := NewCPCPInterceptor()
	if err := interceptor.ValidateToolCall(context.Background(), nil); err == nil {
		t.Fatal("expected error on nil params, got nil")
	}

	emptyParams := &ToolCallParams{Name: ""}
	if err := interceptor.ValidateToolCall(context.Background(), emptyParams); err == nil {
		t.Fatal("expected error on empty tool name, got nil")
	}
}

func TestCPCPInterceptor_SafeToolCall(t *testing.T) {
	t.Parallel()

	interceptor := NewCPCPInterceptor()
	safeParams := &ToolCallParams{
		Name: "write_to_file",
		Arguments: map[string]any{
			"TargetFile": "/workspace/pkg/models/user.go",
			"CodeContent": "package models\n",
		},
	}

	if err := interceptor.ValidateToolCall(context.Background(), safeParams); err != nil {
		t.Fatalf("expected safe tool call to succeed, got: %v", err)
	}
}

func TestCPCPInterceptor_BlockedProcessWrite(t *testing.T) {
	t.Parallel()

	interceptor := NewCPCPInterceptor()

	blockedPaths := []string{
		"/workspace/.zqk/process/backlog_items/BLI-001.yaml",
		"./.zqk/process/requirements/REQ-001.yaml",
		".zqk/streams/change_journal_entry/001.json",
		"/repo/.zqk/cas/objects/ab/123",
		".zqk/wal/log.wal",
	}

	for _, p := range blockedPaths {
		params := &ToolCallParams{
			Name: "write_to_file",
			Arguments: map[string]any{
				"TargetFile": p,
			},
		}

		err := interceptor.ValidateToolCall(context.Background(), params)
		if err == nil {
			t.Errorf("expected path %q to be blocked by CPCP interceptor, but got nil", p)
			continue
		}
		if !strings.Contains(err.Error(), "CPCP-MEMBRANE-001 invariant violation") {
			t.Errorf("expected CPCP violation error for path %q, got: %v", p, err)
		}
	}
}

func TestCPCPInterceptor_ReadToolCallAllowed(t *testing.T) {
	t.Parallel()

	interceptor := NewCPCPInterceptor()
	readParams := &ToolCallParams{
		Name: "view_file",
		Arguments: map[string]any{
			"AbsolutePath": "/workspace/.zqk/process/backlog_items/BLI-001.yaml",
		},
	}

	if err := interceptor.ValidateToolCall(context.Background(), readParams); err != nil {
		t.Fatalf("expected read tool to be allowed on protected path, got: %v", err)
	}
}
