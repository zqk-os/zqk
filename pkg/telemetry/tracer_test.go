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

func TestChildSpan(t *testing.T) {
	parent := NewTraceContext()
	child := parent.ChildSpan()

	if child.TraceID != parent.TraceID {
		t.Errorf("child trace ID %s must match parent trace ID %s", child.TraceID, parent.TraceID)
	}
	if child.ParentSpanID != parent.SpanID {
		t.Errorf("child ParentSpanID %s must match parent SpanID %s", child.ParentSpanID, parent.SpanID)
	}
	if child.SpanID == parent.SpanID {
		t.Error("child SpanID must be unique and distinct from parent SpanID")
	}
	if len(child.SpanID) != 16 {
		t.Errorf("child SpanID length expected 16, got %d", len(child.SpanID))
	}
	if child.Sampled != parent.Sampled {
		t.Errorf("child Sampled %v must match parent Sampled %v", child.Sampled, parent.Sampled)
	}
}
