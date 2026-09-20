package shockwave

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ShockwaveEvent encapsulates a lifecycle transition or kernel shockwave signal.
type ShockwaveEvent struct {
	Topic     string
	SourceID  string
	Timestamp time.Time
	Payload   any
}

// SubscriberFunc is a callback invoked when a matching shockwave event is delivered.
type SubscriberFunc func(evt ShockwaveEvent)

// SubscriptionHandle provides an unregistration mechanism for an active subscription.
type SubscriptionHandle interface {
	Unsubscribe()
}

// PipelineConfig configures the event pipeline buffer and behavior.
type PipelineConfig struct {
	BufferSize     int
	DropOnOverflow bool
	WorkerCount    int
}

type pipelineSub struct {
	id      int64
	pattern string
	fn      SubscriberFunc
}

type subHandle struct {
	id       int64
	pipeline *EventPipeline
}

func (h *subHandle) Unsubscribe() {
	if h.pipeline != nil {
		h.pipeline.removeSubscriber(h.id)
	}
}

// EventPipeline provides decoupled asynchronous pub/sub event distribution.
type EventPipeline struct {
	config       PipelineConfig
	inbox        chan ShockwaveEvent
	subscribers  map[int64]*pipelineSub
	subMu        sync.RWMutex
	subIDSeq     atomic.Int64
	droppedCount atomic.Int64
	logger       logging.Logger
}

var (
	globalEventPipeline     *EventPipeline
	globalEventPipelineOnce sync.Once
)

// GetGlobalEventPipeline returns the system-wide singleton EventPipeline.
func GetGlobalEventPipeline() *EventPipeline {
	globalEventPipelineOnce.Do(func() {
		globalEventPipeline = NewEventPipeline(PipelineConfig{
			BufferSize:     1024,
			DropOnOverflow: true,
			WorkerCount:    4,
		})
	})
	return globalEventPipeline
}

// NewEventPipeline creates an initialized EventPipeline.
func NewEventPipeline(cfg PipelineConfig) *EventPipeline {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 256
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 1
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	return &EventPipeline{
		config:      cfg,
		inbox:       make(chan ShockwaveEvent, cfg.BufferSize),
		subscribers: make(map[int64]*pipelineSub),
		logger:      logger,
	}
}

// Subscribe registers a subscriber callback for the given topic pattern.
// Patterns support "*" (match all), "prefix.*" (prefix match), or exact string matching.
func (p *EventPipeline) Subscribe(pattern string, fn SubscriberFunc) SubscriptionHandle {
	if pattern == "" {
		pattern = "*"
	}

	id := p.subIDSeq.Add(1)
	sub := &pipelineSub{
		id:      id,
		pattern: pattern,
		fn:      fn,
	}

	p.subMu.Lock()
	defer p.subMu.Unlock()
	p.subscribers[id] = sub

	return &subHandle{
		id:       id,
		pipeline: p,
	}
}

func (p *EventPipeline) removeSubscriber(id int64) {
	p.subMu.Lock()
	defer p.subMu.Unlock()
	delete(p.subscribers, id)
}

// Publish enqueues a shockwave event. If the buffer is full and DropOnOverflow is true,
// the event is dropped and false is returned.
func (p *EventPipeline) Publish(evt ShockwaveEvent) bool {
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}

	select {
	case p.inbox <- evt:
		return true
	default:
		p.droppedCount.Add(1)
		return false
	}
}

// DroppedCount returns the total number of events dropped due to buffer overflow.
func (p *EventPipeline) DroppedCount() int64 {
	return p.droppedCount.Load()
}

// Run executes the event distribution worker loop until ctx is cancelled, then drains remaining events.
func (p *EventPipeline) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Drain remaining in-flight events before exiting
			p.drain()
			return
		case evt := <-p.inbox:
			p.dispatch(evt)
		}
	}
}

func (p *EventPipeline) drain() {
	for {
		select {
		case evt := <-p.inbox:
			p.dispatch(evt)
		default:
			return
		}
	}
}

func (p *EventPipeline) dispatch(evt ShockwaveEvent) {
	p.subMu.RLock()
	subs := make([]*pipelineSub, 0, len(p.subscribers))
	for _, sub := range p.subscribers {
		if matchTopic(sub.pattern, evt.Topic) {
			subs = append(subs, sub)
		}
	}
	p.subMu.RUnlock()

	for _, sub := range subs {
		if sub.fn != nil {
			sub.fn(evt)
		}
	}
}

func matchTopic(pattern, topic string) bool {
	if pattern == "*" || pattern == topic {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(topic, prefix+".")
	}
	return false
}
