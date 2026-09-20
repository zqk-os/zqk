package telemetry

import (
	"context"
	"testing"
)

func TestTraceparentFormattingAndParsing(t *testing.T) {
	tc := NewTraceContext()
	header := tc.FormatTraceparent()

	parsed, err := ParseTraceparent(header)
	if err != nil {
		t.Fatalf("failed to parse formatted traceparent: %v", err)
	}

	if parsed.TraceID != tc.TraceID {
		t.Errorf("expected traceID %s, got %s", tc.TraceID, parsed.TraceID)
	}
	if parsed.SpanID != tc.SpanID {
		t.Errorf("expected spanID %s, got %s", tc.SpanID, parsed.SpanID)
	}
	if parsed.Sampled != tc.Sampled {
		t.Errorf("expected sampled %v, got %v", tc.Sampled, parsed.Sampled)
	}
}

func TestSpanCreation(t *testing.T) {
	ctx := context.Background()
	ctx, span := StartSpan(ctx, "mcp.tool_call")
	if span == nil {
		t.Fatal("expected non-nil span")
	}

	span.SetTag("tool", "system_status")
	span.End()

	tc := GetTraceContext(ctx)
	if tc.SpanID != span.TraceCtx.SpanID {
		t.Errorf("context trace context mismatch: expected %s, got %s", span.TraceCtx.SpanID, tc.SpanID)
	}
}
