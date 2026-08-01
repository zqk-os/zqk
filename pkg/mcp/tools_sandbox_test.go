package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

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
