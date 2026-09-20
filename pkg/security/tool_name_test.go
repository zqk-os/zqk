package security

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateToolName_Valid(t *testing.T) {
	validNames := []string{
		"read_file",
		"write_to_file",
		"run-command",
		"mcp:tool.action",
		"tool123",
		"zqk_system_align",
		"A_B-C.D:E",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			if err := ValidateToolName(name); err != nil {
				t.Errorf("expected tool name %q to be valid, got error: %v", name, err)
			}
			if !IsValidToolName(name) {
				t.Errorf("expected IsValidToolName(%q) to be true", name)
			}
		})
	}
}

func TestValidateToolName_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		reason   string
	}{
		{
			name:     "empty string",
			toolName: "",
			reason:   "empty",
		},
		{
			name:     "whitespace only",
			toolName: "   ",
			reason:   "empty",
		},
		{
			name:     "tool name too long",
			toolName: strings.Repeat("a", 257),
			reason:   "length",
		},
		{
			name:     "command injection characters semicolon",
			toolName: "read_file;rm -rf /",
			reason:   "charset",
		},
		{
			name:     "command injection characters pipe",
			toolName: "read_file|cat",
			reason:   "charset",
		},
		{
			name:     "shell backtick injection",
			toolName: "read_`whoami`",
			reason:   "charset",
		},
		{
			name:     "shell variable expansion",
			toolName: "read_$HOME",
			reason:   "charset",
		},
		{
			name:     "space in tool name",
			toolName: "read file",
			reason:   "charset",
		},
		{
			name:     "null byte injection",
			toolName: "read_file\x00extra",
			reason:   "charset",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateToolName(tc.toolName)
			if err == nil {
				t.Errorf("expected error for invalid tool name %q, got nil", tc.toolName)
			}
			if !errors.Is(err, ErrMCPInvalidToolName) {
				t.Errorf("expected ErrMCPInvalidToolName, got %v", err)
			}
			if IsValidToolName(tc.toolName) {
				t.Errorf("expected IsValidToolName(%q) to be false", tc.toolName)
			}
		})
	}
}
