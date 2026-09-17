package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
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

func TestFormatHandler_SanitizesNonFiniteFloats(t *testing.T) {
	t.Parallel()

	h := GetFormatHandler(FormatJSON)
	if h == nil {
		t.Fatal("json handler nil")
	}

	payload := map[string]any{
		"inf":      math.Inf(1),
		"neg_inf":  math.Inf(-1),
		"nan":      math.NaN(),
		"regular":  42.5,
		"nested": map[string]any{
			"deep_inf": math.Inf(1),
		},
		"slice": []any{math.NaN(), math.Inf(-1)},
	}

	// Validate must not fail with "json: unsupported value: +Inf"
	if err := h.Validate(payload); err != nil {
		t.Fatalf("Validate failed on non-finite floats: %v", err)
	}

	out, err := h.Format(payload)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	var round map[string]any
	if err := json.Unmarshal(out, &round); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if round["inf"] != 0.0 || round["neg_inf"] != 0.0 || round["nan"] != 0.0 {
		t.Fatalf("expected non-finite floats sanitized to 0.0, got: %#v", round)
	}
	if round["regular"] != 42.5 {
		t.Fatalf("expected regular float preserved, got: %v", round["regular"])
	}
	nested, ok := round["nested"].(map[string]any)
	if !ok || nested["deep_inf"] != 0.0 {
		t.Fatalf("expected nested inf sanitized to 0.0, got: %#v", nested)
	}
}

// TRACK: BLI-CEF-R28-USA-CLI-ERROR-001 / F-USA-QWEN-001
func TestFormatOutput_JSONErrorUnifiedStructure(t *testing.T) {
	t.Parallel()
	h := GetFormatHandler(FormatJSON)
	if h == nil {
		t.Fatal("json handler nil")
	}

	testErr := errors.New("sample error message")
	out, err := h.Format(testErr)
	if err != nil {
		t.Fatalf("format error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("failed to parse JSON error output: %v\nOutput: %s", err, string(out))
	}

	if parsed["status"] != "error" {
		t.Errorf("expected status 'error', got %v", parsed["status"])
	}
	if parsed["error"] != "sample error message" {
		t.Errorf("expected error message 'sample error message', got %v", parsed["error"])
	}
}

func TestFormatOutput_JSONRPCErrorUnifiedStructure(t *testing.T) {
	t.Parallel()
	h := GetFormatHandler(FormatJSONRPC)
	if h == nil {
		t.Fatal("jsonrpc handler nil")
	}

	testErr := errors.New("sample rpc error")
	out, err := h.Format(testErr)
	if err != nil {
		t.Fatalf("format error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("failed to parse JSON-RPC error output: %v\nOutput: %s", err, string(out))
	}

	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc '2.0', got %v", parsed["jsonrpc"])
	}
	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %T (%v)", parsed["error"], parsed["error"])
	}
	if errObj["message"] != "sample rpc error" {
		t.Errorf("expected error message 'sample rpc error', got %v", errObj["message"])
	}
}
