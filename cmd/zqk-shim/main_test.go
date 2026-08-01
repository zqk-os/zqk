package main

import (
	"testing"
)

func TestIsMutatingOperation(t *testing.T) {
	tests := []struct {
		cmdName  string
		args     []string
		expected bool
	}{
		{"git", []string{"commit", "-m", "msg"}, true},
		{"git", []string{"status"}, false},
		{"gh", []string{"pr", "create"}, true},
		{"gh", []string{"issue", "list"}, false},
		{"other", []string{"commit"}, false},
	}

	for _, tt := range tests {
		result := isMutatingOperation(tt.cmdName, tt.args)
		if result != tt.expected {
			t.Errorf("isMutatingOperation(%q, %v) = %v; want %v", tt.cmdName, tt.args, result, tt.expected)
		}
	}
}
