package infrastructure

import (
	"context"
	"sync"
)

// ShadowSpine implements SpinalSpine but isolates Publish calls to a shadow log.
// It acts as a 'Rubber Room' for shielded handlers, allowing them to see
// events from the main spine but preventing them from affecting it.
type ShadowSpine struct {
	inner        SpinalSpine
	shadowEvents []Event
	mu           sync.Mutex
}

// NewShadowSpine creates a new ShadowSpine wrapping an existing one.
func NewShadowSpine(inner SpinalSpine) *ShadowSpine {
	return &ShadowSpine{
		inner: inner,
	}
}

// Publish implements SpinalSpine but diverts events to the shadow log.
func (s *ShadowSpine) Publish(ctx context.Context, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 'Rubber Room' isolation: log to shadow instead of main spine.
	s.shadowEvents = append(s.shadowEvents, event)
	return nil
}

// Subscribe implements SpinalSpine by passing through to the inner spine.
func (s *ShadowSpine) Subscribe(ctx context.Context, kind string, handler Handler) error {
	return s.inner.Subscribe(ctx, kind, handler)
}

func (s *ShadowSpine) Replay(ctx context.Context, appliedSeq int64, handler Handler) error {
	return s.inner.Replay(ctx, appliedSeq, handler)
}

// Close implements SpinalSpine by closing the inner spine.
func (s *ShadowSpine) Close() error {
	return s.inner.Close()
}

// GetShadowEvents returns a copy of the events published to the shadow log.
func (s *ShadowSpine) GetShadowEvents() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := make([]Event, len(s.shadowEvents))
	copy(events, s.shadowEvents)
	return events
}
