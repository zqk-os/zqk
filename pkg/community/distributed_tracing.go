package community

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrInvalidTraceContext is returned when a trace context cannot be parsed.
	ErrInvalidTraceContext = errors.New("invalid trace context")
	// ErrSpanNotStarted is returned when operations are performed on an unstarted span.
	ErrSpanNotStarted = errors.New("span not started")
	// ErrSpanAlreadyEnded is returned when a span has already been finalized.
	ErrSpanAlreadyEnded = errors.New("span already ended")
)

// TraceID represents a 16-byte unique identifier for a distributed trace.
type TraceID [16]byte

// SpanID represents an 8-byte unique identifier for an individual span.
type SpanID [8]byte

// String returns the 32-character lowercase hex string representation of the TraceID.
func (t TraceID) String() string {
	return hex.EncodeToString(t[:])
}

// String returns the 16-character lowercase hex string representation of the SpanID.
func (s SpanID) String() string {
	return hex.EncodeToString(s[:])
}

// TraceContext represents the distributed tracing propagation context.
type TraceContext struct {
	TraceID    TraceID
	SpanID     SpanID
	TraceFlags byte
}

// String formats the TraceContext as a standard W3C traceparent header.
// Format: 00-{trace_id}-{span_id}-{flags}
func (tc TraceContext) String() string {
	return fmt.Sprintf("00-%s-%s-%02x", tc.TraceID.String(), tc.SpanID.String(), tc.TraceFlags)
}

// ParseTraceContext parses a standard W3C traceparent header string.
func ParseTraceContext(raw string) (TraceContext, error) {
	trimmed := strings.TrimSpace(raw)
	parts := strings.Split(trimmed, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return TraceContext{}, ErrInvalidTraceContext
	}

	if len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return TraceContext{}, ErrInvalidTraceContext
	}

	traceBytes, err := hex.DecodeString(parts[1])
	if err != nil {
		return TraceContext{}, ErrInvalidTraceContext
	}

	spanBytes, err := hex.DecodeString(parts[2])
	if err != nil {
		return TraceContext{}, ErrInvalidTraceContext
	}

	flagsBytes, err := hex.DecodeString(parts[3])
	if err != nil {
		return TraceContext{}, ErrInvalidTraceContext
	}

	var tc TraceContext
	copy(tc.TraceID[:], traceBytes)
	copy(tc.SpanID[:], spanBytes)
	tc.TraceFlags = flagsBytes[0]

	return tc, nil
}

// SpanEvent represents a timestamped event logged within a span.
type SpanEvent struct {
	Name      string
	Timestamp time.Time
	Payload   map[string]string
}

// Span represents an active or completed execution span within a trace.
type Span struct {
	mu        sync.RWMutex
	name      string
	context   TraceContext
	parentID  SpanID
	startTime time.Time
	endTime   time.Time
	tags      map[string]string
	events    []SpanEvent
	isEnded   bool
	isStarted bool
}

// NewTraceContext generates a new TraceContext with fresh random IDs.
func NewTraceContext() (TraceContext, error) {
	var tc TraceContext
	if _, err := rand.Read(tc.TraceID[:]); err != nil {
		return TraceContext{}, err
	}
	if _, err := rand.Read(tc.SpanID[:]); err != nil {
		return TraceContext{}, err
	}
	tc.TraceFlags = 0x01 // recorded flag
	return tc, nil
}

// StartSpan creates and begins a new span under the given name and parent context.
func StartSpan(name string, parent *TraceContext) (*Span, error) {
	if strutil.IsBlank(name) {
		return nil, ErrInvalidTraceContext
	}

	var tc TraceContext
	var parentID SpanID

	if parent != nil {
		tc.TraceID = parent.TraceID
		parentID = parent.SpanID
		if _, err := rand.Read(tc.SpanID[:]); err != nil {
			return nil, err
		}
		tc.TraceFlags = parent.TraceFlags
	} else {
		fresh, err := NewTraceContext()
		if err != nil {
			return nil, err
		}
		tc = fresh
	}

	return &Span{
		name:      name,
		context:   tc,
		parentID:  parentID,
		startTime: time.Now(),
		tags:      make(map[string]string),
		events:    make([]SpanEvent, 0),
		isStarted: true,
	}, nil
}

// Context returns the span's trace context.
func (s *Span) Context() TraceContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.context
}

func (s *Span) withActiveSpanLocked(op func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isStarted {
		return ErrSpanNotStarted
	}
	if s.isEnded {
		return ErrSpanAlreadyEnded
	}
	return op()
}

// SetTag adds or updates a key-value attribute tag on the span.
func (s *Span) SetTag(key, value string) error {
	return s.withActiveSpanLocked(func() error {
		s.tags[key] = value
		return nil
	})
}

// AddEvent logs a timestamped event into the span.
func (s *Span) AddEvent(name string, payload map[string]string) error {
	return s.withActiveSpanLocked(func() error {
		event := SpanEvent{
			Name:      name,
			Timestamp: time.Now(),
			Payload:   make(map[string]string),
		}
		for k, v := range payload {
			event.Payload[k] = v
		}
		s.events = append(s.events, event)
		return nil
	})
}

// End completes the span, capturing execution duration.
func (s *Span) End() (time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isStarted {
		return 0, ErrSpanNotStarted
	}
	if s.isEnded {
		return 0, ErrSpanAlreadyEnded
	}

	s.endTime = time.Now()
	s.isEnded = true
	return s.endTime.Sub(s.startTime), nil
}

// Duration returns the span execution duration.
func (s *Span) Duration() (time.Duration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.isStarted {
		return 0, ErrSpanNotStarted
	}
	if !s.isEnded {
		return time.Since(s.startTime), nil
	}
	return s.endTime.Sub(s.startTime), nil
}
