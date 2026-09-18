package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestFileSandboxRoot_prefersAgentWorktree(t *testing.T) {
	t.Setenv(zqkenv.AgentWorktreeRoot().Name(), "/tmp/atk-wt")
	if got := fileSandboxRoot("/studio"); got != "/tmp/atk-wt" {
		t.Fatalf("fileSandboxRoot=/studio with worktree env = %q", got)
	}
}

func TestFileSandboxRoot_fallsBackToProjectRoot(t *testing.T) {
	t.Setenv(zqkenv.AgentWorktreeRoot().Name(), "")
	if got := fileSandboxRoot("/studio"); got != "/studio" {
		t.Fatalf("fileSandboxRoot=/studio = %q", got)
	}
}

func TestHandleAgentExecuteBashTool(t *testing.T) {
	server := NewServer()
	ctx := context.Background()

	tests := []struct {
		name        string
		args        map[string]any
		expectErr   bool
		errContains string
		expectOut   string
	}{
		{
			name:        "missing command",
			args:        map[string]any{},
			expectErr:   true,
			errContains: "command is required",
		},
		{
			name:        "empty command",
			args:        map[string]any{objects.FieldKeyCommand: ""},
			expectErr:   true,
			errContains: "command is required",
		},
		{
			name:      "successful bash command",
			args:      map[string]any{objects.FieldKeyCommand: "echo 'hello swarm'"},
			expectErr: false,
			expectOut: "hello swarm",
		},
		{
			name:      "failing bash command",
			args:      map[string]any{objects.FieldKeyCommand: "ls /nonexistent_dir_for_test"},
			expectErr: false, // The tool itself succeeds in running bash and returns the stderr in the result payload
			expectOut: "No such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := server.handleAgentExecuteBashTool(ctx, tt.args)

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Errorf("expected non-nil result")
				}
				if tt.expectOut != "" {
					resultStr, ok := result.(string)
					if !ok {
						t.Errorf("expected string result")
					} else if !strings.Contains(resultStr, tt.expectOut) {
						t.Errorf("expected output containing %q, got %q", tt.expectOut, resultStr)
					}
				}
			}
		})
	}
}

func TestAgentShellGuard(t *testing.T) {
	server := NewServer()
	ctx := context.Background()

	forbiddenPaths := []string{
		".zqk/process/goals/test.yaml",
		".zqk/object_drafts/test.yaml",
		"./.zqk/process/test.yaml",
		"../zqk/.zqk/process/test.yaml", // some traversal
	}

	for _, path := range forbiddenPaths {
		t.Run("write_file_"+path, func(t *testing.T) {
			args := map[string]any{
				objects.FieldKeyPath:    path,
				objects.FieldKeyContent: "test",
			}
			_, err := server.handleAgentWriteFileTool(ctx, args)
			if err == nil {
				t.Errorf("expected error for forbidden path %q", path)
			} else if !strings.Contains(err.Error(), "access denied") {
				t.Errorf("expected 'access denied' error, got %q", err.Error())
			}
		})

		t.Run("write_code_"+path, func(t *testing.T) {
			args := map[string]any{
				objects.FieldKeyPath:    path,
				objects.FieldKeyContent: "test",
			}
			_, err := server.handleAgentWriteCodeTool(ctx, args)
			if err == nil {
				t.Errorf("expected error for forbidden path %q", path)
			} else if !strings.Contains(err.Error(), "access denied") {
				t.Errorf("expected 'access denied' error, got %q", err.Error())
			}
		})

		t.Run("bash_command_"+path, func(t *testing.T) {
			args := map[string]any{
				objects.FieldKeyCommand: "echo 'test' > " + path,
			}
			_, err := server.handleAgentExecuteBashTool(ctx, args)
			if err == nil {
				t.Errorf("expected error for forbidden bash command %q", args[objects.FieldKeyCommand])
			} else if !strings.Contains(err.Error(), "access denied") {
				t.Errorf("expected 'access denied' error, got %q", err.Error())
			}
		})
	}
}
