package objects

import (
	"strings"
	"testing"
)

func TestFormatMultiLineYAML(t *testing.T) {
	data := map[string]any{
		"single_line": "hello world",
		"multi_line":  "line one\nline two\nline three",
		"number":      123,
		"flag":        true,
		"nested_map": map[string]any{
			"inner_multiline": "alpha\nbeta",
			"simple":          "simple_val",
		},
		"any_slice": []any{
			"elem1\nmultiline",
			"elem2",
		},
		"string_slice": []string{
			"str1\nmultiline",
			"str2",
		},
	}

	bytes, err := FormatMultiLineYAML(data)
	if err != nil {
		t.Fatalf("FormatMultiLineYAML returned unexpected error: %v", err)
	}

	yamlStr := string(bytes)
	if !strings.Contains(yamlStr, "single_line: hello world") {
		t.Errorf("expected single_line in output, got: %s", yamlStr)
	}
	// Literal block indicator '|' should appear for strings with newlines
	if !strings.Contains(yamlStr, "|") {
		t.Errorf("expected literal style '|' for multiline strings, got: %s", yamlStr)
	}
}

func TestContainsNewline(t *testing.T) {
	if containsNewline("hello") {
		t.Error("containsNewline('hello') = true, want false")
	}
	if !containsNewline("hello\nworld") {
		t.Error("containsNewline('hello\\nworld') = false, want true")
	}
	if !containsNewline("hello\rworld") {
		t.Error("containsNewline('hello\\rworld') = false, want true")
	}
}
