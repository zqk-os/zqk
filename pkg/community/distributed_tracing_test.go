package community

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestTraceContext_NewAndFormat(t *testing.T) {
	tc, err := NewTraceContext()
	if err != nil {
		t.Fatalf("unexpected error creating trace context: %v", err)
	}

	formatted := tc.String()
	parsed, err := ParseTraceContext(formatted)
	if err != nil {
		t.Fatalf("failed to parse formatted trace context: %v", err)
	}

	if parsed.TraceID != tc.TraceID {
		t.Errorf("expected TraceID %s, got %s", tc.TraceID, parsed.TraceID)
	}
	if parsed.SpanID != tc.SpanID {
		t.Errorf("expected SpanID %s, got %s", tc.SpanID, parsed.SpanID)
	}
	if parsed.TraceFlags != tc.TraceFlags {
		t.Errorf("expected TraceFlags %02x, got %02x", tc.TraceFlags, parsed.TraceFlags)
	}
}

func TestSpan_LifecycleAndEvents(t *testing.T) {
	span, err := StartSpan("provider.execute", nil)
	if err != nil {
		t.Fatalf("failed to start span: %v", err)
	}

	if err := span.SetTag("provider.name", "docker-runner"); err != nil {
		t.Fatalf("failed to set tag: %v", err)
	}

	if err := span.AddEvent("cache_miss", map[string]string{"key": "test_image"}); err != nil {
		t.Fatalf("failed to add event: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	duration, err := span.End()
	if err != nil {
		t.Fatalf("failed to end span: %v", err)
	}
	if duration <= 0 {
		t.Errorf("expected positive duration, got %v", duration)
	}

	// Double End should fail
	if _, err := span.End(); err != ErrSpanAlreadyEnded {
		t.Errorf("expected ErrSpanAlreadyEnded, got %v", err)
	}

	// Mutating ended span should fail
	if err := span.SetTag("late", "tag"); err != ErrSpanAlreadyEnded {
		t.Errorf("expected ErrSpanAlreadyEnded on SetTag, got %v", err)
	}
}

func TestSpan_ChildContextPropagation(t *testing.T) {
	parentSpan, err := StartSpan("parent.op", nil)
	if err != nil {
		t.Fatalf("failed to start parent span: %v", err)
	}
	parentCtx := parentSpan.Context()

	childSpan, err := StartSpan("child.op", &parentCtx)
	if err != nil {
		t.Fatalf("failed to start child span: %v", err)
	}

	childCtx := childSpan.Context()
	if childCtx.TraceID != parentCtx.TraceID {
		t.Errorf("expected child TraceID %s to match parent %s", childCtx.TraceID, parentCtx.TraceID)
	}
	if childCtx.SpanID == parentCtx.SpanID {
		t.Errorf("expected child SpanID to be unique, got same as parent: %s", childCtx.SpanID)
	}
}

func TestTraceContext_AdversarialInputs(t *testing.T) {
	invalidCases := []string{
		"",
		"not-a-trace-context",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",  // bad version
		"00-short-00f067aa0ba902b7-01",                             // short trace ID
		"00-4bf92f3577b34da6a3ce929d0e0e4736-short-01",             // short span ID
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-xyz", // bad flags
	}

	for _, tc := range invalidCases {
		if _, err := ParseTraceContext(tc); err != ErrInvalidTraceContext {
			t.Errorf("expected ErrInvalidTraceContext for %q, got %v", tc, err)
		}
	}

	// Empty name span start
	if _, err := StartSpan("", nil); err != ErrInvalidTraceContext {
		t.Errorf("expected ErrInvalidTraceContext for empty name, got %v", err)
	}
}

func TestDistributedTracing_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_DISTRIBUTED_TRACING_AND_OPENTELEMETRY_BRIDGE_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0058 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0058 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0058 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
