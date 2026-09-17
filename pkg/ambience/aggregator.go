package ambience

import (
	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// SignalAggregator implements AnticipatoryEngine but aggregates multiple events
// over a time window to produce a higher confidence intent.
type SignalAggregator struct {
	mesh       EventMesh
	mu         sync.Mutex
	events     []AmbientEvent
	window     time.Duration
	flushCount int
	cancel     context.CancelFunc
	running    bool
	flushCh    chan []AmbientEvent
}

func NewSignalAggregator(mesh EventMesh, window time.Duration, flushCount int) *SignalAggregator {
	return &SignalAggregator{
		mesh:       mesh,
		window:     window,
		flushCount: flushCount,
		flushCh:    make(chan []AmbientEvent, 10),
	}
}

func (s *SignalAggregator) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	ch, err := s.mesh.Subscribe(ctx, []EventType{EventFileModified, EventFocusChanged, EventTestFailed})
	if err != nil {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}

	goroutinelabels.NewGoroutine("ambience.aggregator_loop", "aggregating ambient signals").StartSimple(func() {
		s.loop(ctx, ch)
	})
	return nil
}

func (s *SignalAggregator) loop(ctx context.Context, ch <-chan AmbientEvent) {
	ticker := time.NewTicker(s.window)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			s.mu.Lock()
			s.events = append(s.events, ev)
			if len(s.events) >= s.flushCount {
				eventsToFlush := s.events
				s.events = nil
				s.mu.Unlock()
				s.flush(eventsToFlush)
			} else {
				s.mu.Unlock()
			}
		case <-ticker.C:
			s.mu.Lock()
			if len(s.events) > 0 {
				eventsToFlush := s.events
				s.events = nil
				s.mu.Unlock()
				s.flush(eventsToFlush)
			} else {
				s.mu.Unlock()
			}
		}
	}
}

func (s *SignalAggregator) flush(events []AmbientEvent) {
	select {
	case s.flushCh <- events:
	default:
	}
}

func (s *SignalAggregator) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running && s.cancel != nil {
		s.cancel()
		s.running = false
	}
	return nil
}

func (s *SignalAggregator) PredictIntent(event AmbientEvent) (Intent, error) {
	// Dummy implementation for AnticipatoryEngine
	return Intent{}, nil
}

// Aggregated returns a channel that receives aggregated slices of events.
func (s *SignalAggregator) Aggregated() <-chan []AmbientEvent {
	return s.flushCh
}
