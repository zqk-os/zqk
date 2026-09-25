package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TraceContext holds distributed tracing identifiers compliant with W3C Trace Context spec.
type TraceContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Sampled      bool
}

type traceKey struct{}

// WithTraceContext injects a TraceContext into the Go context.
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceKey{}, tc)
}

// GetTraceContext extracts the TraceContext from context, or creates a new root context.
func GetTraceContext(ctx context.Context) TraceContext {
	if tc, ok := ctx.Value(traceKey{}).(TraceContext); ok {
		return tc
	}
	return NewTraceContext()
}

// NewTraceContext generates a fresh root trace context.
func NewTraceContext() TraceContext {
	traceBytes := make([]byte, 16)
	spanBytes := make([]byte, 8)
	_, _ = rand.Read(traceBytes)
	_, _ = rand.Read(spanBytes)

	return TraceContext{
		TraceID: hex.EncodeToString(traceBytes),
		SpanID:  hex.EncodeToString(spanBytes),
		Sampled: true,
	}
}

// FormatTraceparent formats the context into a W3C traceparent header: 00-{trace_id}-{span_id}-{flags}.
func (tc TraceContext) FormatTraceparent() string {
	flags := "00"
	if tc.Sampled {
		flags = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", tc.TraceID, tc.SpanID, flags)
}

// ParseTraceparent parses a W3C traceparent header string into a TraceContext.
func ParseTraceparent(header string) (TraceContext, error) {
	parts := strings.Split(strings.TrimSpace(header), "-")
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return NewTraceContext(), fmt.Errorf("invalid traceparent header format: %s", header)
	}

	return TraceContext{
		TraceID: parts[1],
		SpanID:  parts[2],
		Sampled: parts[3] == "01",
	}, nil
}

// ChildSpan generates a new child TraceContext linked to this context as parent.
func (tc TraceContext) ChildSpan() TraceContext {
	childBytes := make([]byte, 8)
	_, _ = rand.Read(childBytes)
	return TraceContext{
		TraceID:      tc.TraceID,
		SpanID:       hex.EncodeToString(childBytes),
		ParentSpanID: tc.SpanID,
		Sampled:      tc.Sampled,
	}
}

// Span represents a single unit of work in distributed tracing.
type Span struct {
	Name      string
	TraceCtx  TraceContext
	StartTime time.Time
	EndTime   time.Time
	Tags      map[string]string
	mu        sync.Mutex
}

// StartSpan starts a child span linked to parent trace context.
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	parent := GetTraceContext(ctx)
	child := parent.ChildSpan()

	span := &Span{
		Name:      name,
		TraceCtx:  child,
		StartTime: time.Now(),
		Tags:      make(map[string]string),
	}

	return WithTraceContext(ctx, child), span
}

// SetTag attaches a key/value tag to the span.
func (s *Span) SetTag(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tags[key] = value
}

// End finishes the span.
func (s *Span) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.EndTime = time.Now()
}
