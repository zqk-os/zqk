package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
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
