package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// TestFormatOutput_YAMLEmitsNativeYAMLDocuments asserts --format yaml uses real YAML encoding
// (not JSON-with-indent, which is valid YAML 1.2 but wrong for operators and hid a regression).
func TestFormatOutput_YAMLEmitsNativeYAMLDocuments(t *testing.T) {
	t.Parallel()
	payload := map[string]any{
		objects.FieldKeyID: "CVS-1",
		"nested":           map[string]any{"k": "v"},
	}
	h := GetFormatHandler(FormatYAML)
	if h == nil {
		t.Fatal("yaml handler nil")
	}
	out, err := h.Format(payload)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	s := strings.TrimSpace(string(out))
	if len(s) == 0 {
		t.Fatal("empty output")
	}
	// Native YAML from gopkg.in/yaml for maps uses "key:" lines, not a top-level brace.
	if s[0] == '{' || s[0] == '[' {
		t.Fatalf("expected YAML document style, got JSON-style first byte %q:\n%s", s[0], s)
	}
	var round map[string]any
	if err := yaml.Unmarshal(out, &round); err != nil {
		t.Fatalf("unmarshal yaml: %v\n%s", err, s)
	}
	if round[objects.FieldKeyID] != "CVS-1" {
		t.Fatalf("round-trip: %#v", round)
	}
}

func TestFormatOutput_JSONEmitsBraceForObject(t *testing.T) {
	t.Parallel()
	payload := map[string]any{"ok": true}
	h := GetFormatHandler(FormatJSON)
	out, err := h.Format(payload)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	s := strings.TrimSpace(string(out))
	if s[0] != '{' {
		t.Fatalf("expected JSON object, got: %s", s)
	}
	if err := json.Unmarshal(out, new(map[string]any)); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
}

func TestTableFormatHandler_UsesYAMLStyleNotJSONBrace(t *testing.T) {
	t.Parallel()
	h := GetFormatHandler(FormatTable)
	out, err := h.Format(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	s := strings.TrimSpace(string(out))
	if len(s) > 0 && (s[0] == '{' || s[0] == '[') {
		t.Fatalf("table format should not emit JSON-style body: %s", s)
	}
}

func TestYAMLFormatHandler_StreamWritesSameFamilyAsFormat(t *testing.T) {
	t.Parallel()
	h := GetFormatHandler(FormatYAML)
	var buf bytes.Buffer
	if err := h.Stream(t.Context(), map[string]any{"x": 1}, &buf); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	s := strings.TrimSpace(buf.String())
	if len(s) > 0 && s[0] == '{' {
		t.Fatalf("stream yaml should not be JSON brace: %s", s)
	}
}
