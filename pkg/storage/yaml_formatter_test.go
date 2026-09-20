package storage

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestFormatMultiLineYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		contains string // Expected to be in output
	}{
		{
			name: "single line string",
			input: map[string]any{
				objects.FieldKeyOperation: "Simple operation",
			},
			contains: "operation: Simple operation",
		},
		{
			name: "multi-line string",
			input: map[string]any{
				objects.FieldKeyOperation: "Line 1\nLine 2\nLine 3",
			},
			contains: "operation: |",
		},
		{
			name: "commit message with newlines",
			input: map[string]any{
				objects.FieldKeyMetadata: map[string]any{
					"commit_message": "feat: Add feature\n\n- Item 1\n- Item 2",
				},
			},
			contains: "commit_message: |",
		},
		{
			name: "nested multi-line strings",
			input: map[string]any{
				objects.FieldKeyOperation: "Multi-line operation:\n  - Step 1\n  - Step 2",
				objects.FieldKeyMetadata: map[string]any{
					objects.FieldKeyDescription: "Description\nwith\nmultiple\nlines",
				},
			},
			contains: "operation: |",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := FormatMultiLineYAML(tt.input)
			if err != nil {
				t.Fatalf("FormatMultiLineYAML() error = %v", err)
			}

			outputStr := string(output)
			if !strings.Contains(outputStr, tt.contains) {
				t.Errorf("FormatMultiLineYAML() output does not contain expected string %q\nGot:\n%s", tt.contains, outputStr)
			}
		})
	}
}

func TestContainsNewline(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"no newline", "simple string", false},
		{"with newline", "line 1\nline 2", true},
		{"with carriage return", "line 1\rline 2", true},
		{"with both", "line 1\r\nline 2", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsNewline(tt.input)
			if result != tt.expected {
				t.Errorf("containsNewline(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
