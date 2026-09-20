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
