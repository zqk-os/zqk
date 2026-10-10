package mcp

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/walutil"
)

const (
	defaultWakerPollInterval = 50 * time.Millisecond
	defaultWakerBufferSize   = 128
	recentEventRetention     = 30 * time.Second
)

// WakerStreamConfig configures the Zero-Idle MCP event stream.
type WakerStreamConfig struct {
	WakerRegistry *lifecycle.JobWakerRegistry
	ProjectRoot   string
	WAL           *lifecycle.LifecycleEventWAL
	PollInterval  time.Duration
	BufferSize    int
}

// WakerStreamOption customizes WakerStreamConfig.
type WakerStreamOption func(*WakerStreamConfig)

// WithJobWakerRegistry sets the JobWakerRegistry source.
func WithJobWakerRegistry(reg *lifecycle.JobWakerRegistry) WakerStreamOption {
	return func(c *WakerStreamConfig) {
		c.WakerRegistry = reg
	}
}

// WithProjectRoot sets the project root for discovering the LifecycleEventWAL.
func WithProjectRoot(root string) WakerStreamOption {
	return func(c *WakerStreamConfig) {
		c.ProjectRoot = root
	}
}

// WithWAL sets an explicit LifecycleEventWAL.
func WithWAL(wal *lifecycle.LifecycleEventWAL) WakerStreamOption {
	return func(c *WakerStreamConfig) {
		c.WAL = wal
	}
}

// WithPollInterval sets the background WAL polling interval.
func WithPollInterval(d time.Duration) WakerStreamOption {
	return func(c *WakerStreamConfig) {
		c.PollInterval = d
	}
}

// WithBufferSize sets the event channel buffer size.
func WithBufferSize(size int) WakerStreamOption {
	return func(c *WakerStreamConfig) {
		c.BufferSize = size
	}
}

// WakerEventStreamer streams real-time JobWakerRegistry dispatches and
// LifecycleEventWAL records to connected MCP clients as JSON-RPC notifications.
type WakerEventStreamer struct {
	server        *Server
	cfg           WakerStreamConfig
	ctx           context.Context
	cancel        context.CancelFunc
	running       atomic.Bool
	streamedTotal atomic.Int64
	droppedTotal  atomic.Int64
	unregisterFn  func()
	recentMu      sync.Mutex
	recentEvents  map[string]time.Time
	wg            sync.WaitGroup
}

// NewWakerEventStreamer creates a new streamer bound to an MCP Server.
func NewWakerEventStreamer(server *Server, opts ...WakerStreamOption) *WakerEventStreamer {
	cfg := WakerStreamConfig{
		WakerRegistry: lifecycle.GetGlobalJobWakerRegistry(),
		PollInterval:  defaultWakerPollInterval,
		BufferSize:    defaultWakerBufferSize,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &WakerEventStreamer{
		server:       server,
		cfg:          cfg,
		recentEvents: make(map[string]time.Time),
	}
}

// Start begins listening to the JobWakerRegistry and LifecycleEventWAL.
func (s *WakerEventStreamer) Start(parentCtx context.Context) error {
	if !s.running.CompareAndSwap(false, true) {
		return errfmt.Errorf("waker event streamer is already running")
	}

	if parentCtx == nil {
		parentCtx = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(parentCtx)

	reg := s.cfg.WakerRegistry
	if reg == nil {
		reg = lifecycle.GetGlobalJobWakerRegistry()
	}

	wakerCh, unreg := reg.RegisterWildcard()
	s.unregisterFn = unreg

	s.wg.Add(1)
	goroutinelabels.NewGoroutine("mcp_waker_stream_listener", "streams job waker events to mcp clients").StartSimple(func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.ctx.Done():
				return
			case ev, ok := <-wakerCh:
				if !ok {
					return
				}
				if ev != nil {
					s.handleLifecycleEvent(ev)
				}
			}
		}
	})

	if s.cfg.WAL != nil || s.cfg.ProjectRoot != "" {
		s.wg.Add(1)
		goroutinelabels.NewGoroutine("mcp_waker_stream_wal_poll", "polls lifecycle WAL for external waker events").StartSimple(func() {
			defer s.wg.Done()
			s.runWALPoller()
		})
	}

	return nil
}

// runWALPoller tails the LifecycleEventWAL and streams events that did not arrive via in-memory fast-path.
func (s *WakerEventStreamer) runWALPoller() {
	wal := s.cfg.WAL
	if wal == nil && s.cfg.ProjectRoot != "" {
		w, err := lifecycle.GetOrCreateLifecycleWAL(s.cfg.ProjectRoot)
		if err == nil {
			wal = w
		}
	}
	if wal == nil {
		return
	}

	interval := s.cfg.PollInterval
	if interval <= 0 {
		interval = defaultWakerPollInterval
	}

	var cursor walutil.ReplayCursor
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			newCursor, err := wal.ReplayFromCursor(cursor, func(ev *lifecycle.LifecycleEvent) error {
				if ev != nil {
					s.handleLifecycleEvent(ev)
				}
				return nil
			})
			if err == nil {
				cursor = newCursor
			}
		}
	}
}

// isDuplicate checks if the event was recently dispatched to avoid double-delivery
// across concurrent in-memory and WAL replay paths.
func (s *WakerEventStreamer) isDuplicate(ev *lifecycle.LifecycleEvent) bool {
	s.recentMu.Lock()
	defer s.recentMu.Unlock()

	now := time.Now()
	if len(s.recentEvents) > 500 {
		for k, t := range s.recentEvents {
			if now.Sub(t) > recentEventRetention {
				delete(s.recentEvents, k)
			}
		}
	}

	key := fmt.Sprintf("%d:%s:%s:%s:%s", ev.Seq, ev.ID, ev.EventType, ev.ToStatus, ev.CriterionID)
	if _, exists := s.recentEvents[key]; exists {
		return true
	}
	s.recentEvents[key] = now
	return false
}

// handleLifecycleEvent translates and streams a LifecycleEvent to MCP clients.
func (s *WakerEventStreamer) handleLifecycleEvent(ev *lifecycle.LifecycleEvent) {
	if ev == nil || s.isDuplicate(ev) {
		return
	}

	eventType := EventTypeTaskWaker
	if ev.EventType == lifecycle.EventTypeStatusTransition || ev.EventType == lifecycle.EventTypeCriterionSatisfied {
		eventType = EventTypeLifecycleEvent
	}

	message := formatWakerMessage(ev)

	fields := map[string]any{
		"job_id":      ev.ID,
		"task_id":     ev.ID,
		"event_type":  string(ev.EventType),
		"kind":        ev.Kind,
		"from_status": ev.FromStatus,
		"to_status":   ev.ToStatus,
		"seq":         ev.Seq,
	}
	if ev.CriterionID != "" {
		fields["criterion_id"] = ev.CriterionID
	}
	if ev.TargetKind != "" {
		fields["target_kind"] = ev.TargetKind
	}
	if ev.TargetID != "" {
		fields["target_id"] = ev.TargetID
	}
	if ev.FieldName != "" {
		fields["field_name"] = ev.FieldName
	}
	if len(ev.Scope) > 0 {
		fields["scope"] = ev.Scope
	}

	mcpEvent := &Event{
		Type:      eventType,
		Timestamp: ev.Timestamp(),
		Message:   message,
		Fields:    fields,
		Severity:  "info",
		Priority:  "high",
	}

	// 1. Emit to EventEmitter subscribers (e.g. notifications/event)
	if s.server != nil && s.server.eventEmitter != nil {
		s.server.eventEmitter.Emit(mcpEvent)
	}

	// 2. Broadcast JSON-RPC notifications/message to all connected client sessions
	if s.server != nil {
		s.server.BroadcastMessage(message, string(eventType), "high")
	}

	s.streamedTotal.Add(1)
}

// formatWakerMessage creates human-readable descriptions of waker and lifecycle updates.
func formatWakerMessage(ev *lifecycle.LifecycleEvent) string {
	switch ev.EventType {
	case lifecycle.EventTypeSchedulerCallback:
		return fmt.Sprintf("Scheduler callback waker event: %s %s status=%s", ev.Kind, ev.ID, ev.ToStatus)
	case lifecycle.EventTypeCriterionSatisfied:
		return fmt.Sprintf("Criterion satisfied waker event: %s", ev.CriterionID)
	case lifecycle.EventTypeStatusTransition:
		return fmt.Sprintf("Status transition waker event: %s %s (%s -> %s)", ev.Kind, ev.ID, ev.FromStatus, ev.ToStatus)
	default:
		return fmt.Sprintf("Lifecycle waker event: %s %s", ev.Kind, ev.ID)
	}
}

// Stop cleanly unregisters all listeners and waits for goroutines to exit.
// Guarantees zero channel and goroutine leaks.
func (s *WakerEventStreamer) Stop() {
	if !s.running.CompareAndSwap(true, false) {
		return
	}

	if s.cancel != nil {
		s.cancel()
	}

	if s.unregisterFn != nil {
		s.unregisterFn()
		s.unregisterFn = nil
	}

	s.wg.Wait()
}

// Stats returns the lifetime number of streamed and dropped events.
func (s *WakerEventStreamer) Stats() (streamed, dropped int64) {
	return s.streamedTotal.Load(), s.droppedTotal.Load()
}

// IsRunning reports whether the waker streamer is actively listening.
func (s *WakerEventStreamer) IsRunning() bool {
	return s.running.Load()
}

// AttachZeroIdleWakerStream attaches and starts a WakerEventStreamer on the MCP server,
// registering an automatic shutdown hook to guarantee clean unregistration on server stop.
func AttachZeroIdleWakerStream(server *Server, projectRoot string, opts ...WakerStreamOption) *WakerEventStreamer {
	if server == nil {
		return nil
	}

	allOpts := append([]WakerStreamOption{WithProjectRoot(projectRoot)}, opts...)
	streamer := NewWakerEventStreamer(server, allOpts...)

	if err := streamer.Start(context.Background()); err != nil {
		return nil
	}

	server.RegisterShutdownHook(func(ctx context.Context) error {
		streamer.Stop()
		return nil
	})

	return streamer
}
