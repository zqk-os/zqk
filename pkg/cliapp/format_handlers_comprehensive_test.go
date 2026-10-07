package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/quality"
)

type mockPermissionChecker struct {
	allowFormat  bool
	formatReason string
	allowData    bool
	dataReason   string
}

func (m *mockPermissionChecker) CheckPermission(ctx context.Context, operation string, resource string) (bool, string) {
	return true, ""
}

func (m *mockPermissionChecker) CheckFormatPermission(ctx context.Context, format OutputFormat) (bool, string) {
	return m.allowFormat, m.formatReason
}

func (m *mockPermissionChecker) CheckDataAccess(ctx context.Context, data any) (bool, string) {
	return m.allowData, m.dataReason
}

func TestFormatHandlers_AllFormats_FormatAndStream(t *testing.T) {
	InitializeDefaultHandlers()

	testData := map[string]any{
		"id":    "GOAL-100",
		"title": "Achieve 80%+ Test Coverage",
		"count": 42,
	}

	handlers := []struct {
		format      OutputFormat
		isStreaming bool
	}{
		{FormatTable, false},
		{FormatJSON, false},
		{FormatJSONL, true},
		{FormatYAML, false},
		{FormatJSONRPC, true},
		{FormatRaw, false},
		{FormatMarkdown, false},
		{FormatHTML, false},
	}

	for _, tc := range handlers {
		t.Run(string(tc.format), func(t *testing.T) {
			h := GetFormatHandler(tc.format)
			if h == nil {
				t.Fatalf("expected handler for format %s", tc.format)
			}
			if h.IsStreaming() != tc.isStreaming {
				t.Errorf("format %s: expected IsStreaming=%v, got %v", tc.format, tc.isStreaming, h.IsStreaming())
			}
			if err := h.Validate(testData); err != nil {
				t.Errorf("format %s: Validate failed: %v", tc.format, err)
			}

			// Test Format
			formatted, err := h.Format(testData)
			if err != nil {
				t.Fatalf("format %s: Format failed: %v", tc.format, err)
			}
			if len(formatted) == 0 {
				t.Errorf("format %s: Format returned empty bytes", tc.format)
			}

			// Test Stream
			var buf bytes.Buffer
			if err := h.Stream(context.Background(), testData, &buf); err != nil {
				t.Fatalf("format %s: Stream failed: %v", tc.format, err)
			}
			if buf.Len() == 0 {
				t.Errorf("format %s: Stream wrote 0 bytes", tc.format)
			}
		})
	}
}

func TestJSONLFormatHandler_SliceAndCancellation(t *testing.T) {
	h := &JSONLFormatHandler{}
	sliceData := []any{
		map[string]any{"id": "BLI-1"},
		map[string]any{"id": "BLI-2"},
	}

	// Format slice
	out, err := h.Format(sliceData)
	if err != nil {
		t.Fatalf("Format slice failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines of JSONL, got %d: %s", len(lines), string(out))
	}

	// Stream with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled immediately
	var buf bytes.Buffer
	err = h.Stream(ctx, sliceData, &buf)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

func TestJSONRPCFormatHandler_ErrorHandling(t *testing.T) {
	h := NewJSONRPCFormatHandler(nil)

	// Format with error
	testErr := errors.New("simulated RPC failure")
	out, err := h.Format(testErr)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "simulated RPC failure") || !strings.Contains(s, "-32000") {
		t.Errorf("expected JSON-RPC error structure, got: %s", s)
	}

	// StreamWithEvents
	var buf bytes.Buffer
	err = h.StreamWithEvents(context.Background(), map[string]any{"ok": true}, &buf, []string{"all"})
	if err != nil {
		t.Fatalf("StreamWithEvents failed: %v", err)
	}
	if !strings.Contains(buf.String(), "2.0") {
		t.Errorf("expected JSON-RPC version 2.0 in stream output: %s", buf.String())
	}
}

func TestFormatHandlerWithPermissions(t *testing.T) {
	inner := &JSONFormatHandler{}
	checker := &mockPermissionChecker{
		allowFormat: true,
		allowData:   true,
	}

	wrapper := NewFormatHandlerWithPermissions(inner, checker)
	if wrapper.IsStreaming() != inner.IsStreaming() {
		t.Errorf("expected delegated IsStreaming")
	}

	// Format delegates
	out, err := wrapper.Format(map[string]any{"test": 1})
	if err != nil || len(out) == 0 {
		t.Errorf("unexpected error on Format: %v", err)
	}

	// Stream allowed
	var buf bytes.Buffer
	err = wrapper.Stream(context.Background(), map[string]any{"test": 1}, &buf)
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	// Deny format permission
	checker.allowFormat = false
	checker.formatReason = "prohibited by policy"
	err = wrapper.Stream(context.Background(), map[string]any{"test": 1}, &buf)
	if err == nil || !strings.Contains(err.Error(), "format not allowed: prohibited by policy") {
		t.Errorf("expected format not allowed error, got: %v", err)
	}

	// Deny data permission
	checker.allowFormat = true
	checker.allowData = false
	checker.dataReason = "unauthorized confidential data"
	err = wrapper.Stream(context.Background(), map[string]any{"test": 1}, &buf)
	if err == nil || !strings.Contains(err.Error(), "data access denied: unauthorized confidential data") {
		t.Errorf("expected data access denied error, got: %v", err)
	}

	// SetPermissionChecker
	newChecker := &mockPermissionChecker{allowFormat: true, allowData: true}
	wrapper.SetPermissionChecker(newChecker)
}

type mockTablePrintable struct {
	content string
}

func (m *mockTablePrintable) FormatTable() ([]byte, error) {
	return []byte(m.content), nil
}

func TestTableFormatHandler_TableFormattingAndFallbacks(t *testing.T) {
	h := &TableFormatHandler{}

	// TablePrintable interface
	tp := &mockTablePrintable{content: "custom-table-output"}
	out, err := h.Format(tp)
	if err != nil || string(out) != "custom-table-output" {
		t.Errorf("expected custom-table-output, got: %q", string(out))
	}

	// Objects map containing []map[string]any with id, title, status, kind
	objData := map[string]any{
		"objects": []map[string]any{
			{"id": "GOAL-1", "title": "First Goal", "status": "active", "kind": "goal"},
			{"id": "GOAL-2", "title": "Second Goal", "status": "planned", "kind": "goal"},
		},
	}
	out, err = h.Format(objData)
	if err != nil || !strings.Contains(string(out), "GOAL-1") || !strings.Contains(string(out), "STATUS") {
		t.Errorf("expected rendered table with headers: %s", string(out))
	}

	// Slice of maps with non-standard fallback columns (first 3 keys)
	nonStd := []map[string]any{
		{"foo": "val1", "bar": "val2", "baz": "val3"},
	}
	out, err = h.Format(nonStd)
	if err != nil || !strings.Contains(string(out), "FOO") {
		t.Errorf("expected fallback column table: %s", string(out))
	}

	// Any slice with all maps
	anySlice := []any{
		map[string]any{"id": "BLI-1", "name": "Item 1"},
	}
	out, err = h.Format(anySlice)
	if err != nil || !strings.Contains(string(out), "BLI-1") {
		t.Errorf("expected rendered table from any slice: %s", string(out))
	}

	// Single map fallback (renders YAML)
	singleMap := map[string]any{"simple_key": "simple_val"}
	out, err = h.Format(singleMap)
	if err != nil || !strings.Contains(string(out), "simple_key: simple_val") {
		t.Errorf("expected yaml fallback: %s", string(out))
	}
}

func TestYAMLFormatHandler_StreamingAndValidation(t *testing.T) {
	h := &YAMLFormatHandler{}
	if h.IsStreaming() {
		t.Errorf("expected IsStreaming=false for YAML")
	}

	var buf bytes.Buffer
	data := map[string]any{"greeting": "hello"}
	if err := h.Stream(context.Background(), data, &buf); err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	if !strings.Contains(buf.String(), "greeting: hello") {
		t.Errorf("expected YAML content: %s", buf.String())
	}
}

func TestProseAndMarkdownRendering_AllVariants(t *testing.T) {
	rawHandler := &RawFormatHandler{}
	mdHandler := &MarkdownFormatHandler{}
	htmlHandler := &HTMLFormatHandler{}

	if rawHandler.IsStreaming() || mdHandler.IsStreaming() || htmlHandler.IsStreaming() {
		t.Errorf("expected IsStreaming=false")
	}
	if err := rawHandler.Validate(nil); err != nil {
		t.Errorf("expected Validate=nil")
	}
	if err := mdHandler.Validate(nil); err != nil {
		t.Errorf("expected Validate=nil")
	}
	if err := htmlHandler.Validate(nil); err != nil {
		t.Errorf("expected Validate=nil")
	}

	// Map with title, subtitle, statement, description, body
	proseData := map[string]any{
		"title":       "Important Notice\\nSecond Line",
		"subtitle":    "Brief subtitle\\tindented",
		"description": "Here is the \\\"quote\\\" and \\u0041 unicode",
	}

	rawOut, err := rawHandler.Format(proseData)
	if err != nil || !strings.Contains(string(rawOut), "Here is the \"quote\" and A unicode") {
		t.Errorf("unexpected raw output: %s", string(rawOut))
	}

	mdOut, err := mdHandler.Format(proseData)
	if err != nil || !strings.Contains(string(mdOut), "Here is the \"quote\" and A unicode") {
		t.Errorf("unexpected markdown output: %s", string(mdOut))
	}

	htmlOut, err := htmlHandler.Format(proseData)
	if err != nil || !strings.Contains(string(htmlOut), "<h1>Important Notice</h1>") {
		t.Errorf("unexpected html output: %s", string(htmlOut))
	}

	// Stream methods
	var b bytes.Buffer
	if err := rawHandler.Stream(context.Background(), "hello", &b); err != nil || b.String() != "hello" {
		t.Errorf("unexpected raw stream: %s", b.String())
	}
	b.Reset()
	if err := mdHandler.Stream(context.Background(), "hello", &b); err != nil || b.String() != "hello" {
		t.Errorf("unexpected md stream: %s", b.String())
	}
	b.Reset()
	if err := htmlHandler.Stream(context.Background(), "# Heading", &b); err != nil || !strings.Contains(b.String(), "<h1>Heading</h1>") {
		t.Errorf("unexpected html stream: %s", b.String())
	}

	// Full markdown coverage: headings, code blocks, lists, horizontal rules, quotes, links
	fullMd := "# H1\n## H2\n### H3\n#### H4\n```\ncode block\n```\n---\n* item 1\n- item 2\n\n> blockquote\n**bold** *italic* `inline` [title](http://example.com)\n"
	b.Reset()
	if err := htmlHandler.Stream(context.Background(), fullMd, &b); err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}
	htmlStr := b.String()
	if !strings.Contains(htmlStr, "<h2>H2</h2>") || !strings.Contains(htmlStr, "<pre><code>code block</code></pre>") || !strings.Contains(htmlStr, "<li>item 1</li>") {
		t.Errorf("expected full html render, got: %s", htmlStr)
	}
}

func TestJSONLFormatHandler_StreamSliceAndObject(t *testing.T) {
	h := &JSONLFormatHandler{}
	if !h.IsStreaming() {
		t.Errorf("expected JSONLFormatHandler.IsStreaming=true")
	}

	// Stream slice of maps
	var buf bytes.Buffer
	sliceData := []any{
		map[string]any{"id": "BLI-1", "val": 10},
		map[string]any{"id": "BLI-2", "val": 20},
	}
	if err := h.Stream(context.Background(), sliceData, &buf); err != nil {
		t.Fatalf("Stream slice failed: %v", err)
	}
	if !strings.Contains(buf.String(), "BLI-1") || !strings.Contains(buf.String(), "BLI-2") {
		t.Errorf("expected stream to contain both objects: %s", buf.String())
	}

	// Stream single object
	buf.Reset()
	singleObj := map[string]any{"id": "GOAL-99", "status": "active"}
	if err := h.Stream(context.Background(), singleObj, &buf); err != nil {
		t.Fatalf("Stream single object failed: %v", err)
	}
	if !strings.Contains(buf.String(), "GOAL-99") {
		t.Errorf("expected stream to contain single object: %s", buf.String())
	}
}

func TestCSVFormatHandler_Comprehensive(t *testing.T) {
	h := &CSVFormatHandler{}
	if h.IsStreaming() {
		t.Errorf("expected CSVFormatHandler.IsStreaming=false")
	}

	// Invalid type
	if err := h.Validate("not a matrix result"); err == nil {
		t.Errorf("expected Validate error for invalid data type")
	}
	if _, err := h.Format("not a matrix result"); err == nil {
		t.Errorf("expected Format error for invalid data type")
	}

	// Valid matrix result
	res := &quality.MatrixGetResult{
		Header: []string{"id", "status"},
		Rows: []map[string]string{
			{"id": "BLI-1", "status": "completed"},
			{"id": "BLI-2", "status": "planned"},
		},
	}
	if err := h.Validate(res); err != nil {
		t.Fatalf("unexpected Validate error: %v", err)
	}

	formatted, err := h.Format(res)
	if err != nil {
		t.Fatalf("unexpected Format error: %v", err)
	}
	csvStr := string(formatted)
	if !strings.Contains(csvStr, "id,status") || !strings.Contains(csvStr, "BLI-1,completed") {
		t.Errorf("unexpected CSV output: %s", csvStr)
	}

	var buf bytes.Buffer
	if err := h.Stream(context.Background(), res, &buf); err != nil {
		t.Fatalf("unexpected Stream error: %v", err)
	}
	if !strings.Contains(buf.String(), "BLI-2,planned") {
		t.Errorf("expected Stream output to contain BLI-2, got: %s", buf.String())
	}
}

func TestFormatHandlerWithPermissions_Comprehensive(t *testing.T) {
	base := &YAMLFormatHandler{}
	checker := &mockPermissionChecker{allowFormat: true, allowData: true}
	h := NewFormatHandlerWithPermissions(base, checker)

	if h.IsStreaming() {
		t.Errorf("expected IsStreaming=false")
	}
	if err := h.Validate(map[string]any{"a": "b"}); err != nil {
		t.Errorf("unexpected Validate error: %v", err)
	}

	out, err := h.Format(map[string]any{"key": "value"})
	if err != nil || !strings.Contains(string(out), "key: value") {
		t.Fatalf("unexpected Format output: %s (err=%v)", string(out), err)
	}

	// Stream with allowed permissions
	var buf bytes.Buffer
	if err := h.Stream(context.Background(), map[string]any{"foo": "bar"}, &buf); err != nil {
		t.Fatalf("unexpected Stream error: %v", err)
	}
	if !strings.Contains(buf.String(), "foo: bar") {
		t.Errorf("expected Stream output, got: %s", buf.String())
	}

	// Stream with format not allowed
	checker.allowFormat = false
	checker.formatReason = "rpc required"
	if err := h.Stream(context.Background(), map[string]any{"foo": "bar"}, &buf); err == nil {
		t.Errorf("expected format not allowed error")
	}

	// Stream with data access denied
	checker.allowFormat = true
	checker.allowData = false
	checker.dataReason = "confidential"
	if err := h.Stream(context.Background(), map[string]any{"foo": "bar"}, &buf); err == nil {
		t.Errorf("expected data access denied error")
	}

	// SetPermissionChecker
	h.SetPermissionChecker(nil)
	buf.Reset()
	if err := h.Stream(context.Background(), map[string]any{"foo": "baz"}, &buf); err != nil {
		t.Fatalf("unexpected Stream error after clearing checker: %v", err)
	}
}
