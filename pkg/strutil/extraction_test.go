package strutil

import (
	"testing"
)

func TestExtractFirstPrefixedValue(t *testing.T) {
	lines := []string{
		"Some random text",
		"CMD: zqk object list --limit 10",
		"VERIFY: zqk test",
	}

	result := ExtractFirstPrefixedValue(lines, "CMD:", "VERIFY:")
	if result != "zqk object list --limit 10" {
		t.Errorf("expected 'zqk object list --limit 10', got '%s'", result)
	}

	result2 := ExtractFirstPrefixedValue(lines, "MISSING:")
	if result2 != "" {
		t.Errorf("expected empty string, got '%s'", result2)
	}
}

func TestFindLineWithPrefix(t *testing.T) {
	lines := []string{
		"Some text here",
		"./bin/zqk do something",
		"zqk another thing",
	}

	result := FindLineWithPrefix(lines, "zqk ", "./bin/zqk ")
	if result != "./bin/zqk do something" {
		t.Errorf("expected './bin/zqk do something', got '%s'", result)
	}
}

func TestIsAllDigits(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"", false},
		{"12345", true},
		{"0", true},
		{"123a45", false},
		{" 123", false},
		{"123 ", false},
		{"-123", false},
	}
	for _, tc := range tests {
		if got := IsAllDigits(tc.input); got != tc.expected {
			t.Errorf("IsAllDigits(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}
