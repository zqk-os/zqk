package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestExtractFirstCompleteJSONObject_bracesInsideString(t *testing.T) {
	// Naive brace counting would close at the first `}` inside prompt_body.
	raw := `{
  "id": "PROMPT-1",
  "prompt_body": "Example: use {foo} and } bar",
  "kind": "prompt_template"
}`

	got := ExtractFirstCompleteJSONObject(raw)
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("extracted value is not valid JSON: %v\n%s", err, got)
	}
	body, _ := m[objects.FieldKeyPromptBody].(string)
	if !strings.Contains(body, "{foo}") {
		t.Fatalf("prompt_body truncated or wrong: %q", body)
	}
}

func TestFilterDebugLogObjectsFromOutput_debugThenResult(t *testing.T) {
	debug := `{"event":"debug","level":"debug"}`
	result := `{"id":"x","prompt_body":"brace { in } string"}`
	combined := debug + "\n" + result

	got := FilterDebugLogObjectsFromOutput(combined)
	if got != strings.TrimSpace(result) {
		t.Fatalf("got %q want %q", got, result)
	}
}

func TestFilterDebugLogObjectsFromOutput_singleObjectWithBracesInString(t *testing.T) {
	one := `{"prompt_body":"a { b } c","x":1}`
	got := FilterDebugLogObjectsFromOutput(one)
	if got != strings.TrimSpace(one) {
		t.Fatalf("got %q want %q", got, one)
	}
}
