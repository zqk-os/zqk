package callback

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

const (
	// SubscriberNameShockwave is the canonical identifier for the shockwave subscriber.
	SubscriberNameShockwave = "shockwave"

	// DefaultShockwaveBufferSize is the default buffer depth for async shockwave dispatching.
	DefaultShockwaveBufferSize = 256

	// DefaultKindSchedulerJob is the fallback kind when dispatching scheduler callbacks without an explicit object kind.
	DefaultKindSchedulerJob = "scheduler_job"

	// DefaultKindUnknown is the fallback kind when an object kind cannot be inferred.
	DefaultKindUnknown = "unknown"

	// FieldKeyObjectID represents the payload field for object identifiers.
	FieldKeyObjectID = "object_id"

	// FieldKeyJobID represents the payload field for scheduler job identifiers.
	FieldKeyJobID = "job_id"

	// FieldKeyID represents the generic payload field for entity identifiers.
	FieldKeyID = "id"

	// FieldKeyOp represents the payload field for mutation operations.
	FieldKeyOp = "op"
)

var (
	// ErrNilCallbackEntry indicates that a nil CallbackEntry was passed for notification.
	ErrNilCallbackEntry = errfmt.Errorf("callback entry cannot be nil")

	// ErrNilPayload indicates that a CallbackEntry has an uninitialized payload map.
	ErrNilPayload = errfmt.Errorf("callback entry payload cannot be nil")

	// ErrNilBus indicates that the underlying InvalidationShockwaveBus is unassigned.
	ErrNilBus = errfmt.Errorf("invalidation shockwave bus cannot be nil")

	// ErrSubscriberClosed indicates an operation was attempted on a closed ShockwaveSubscriber.
	ErrSubscriberClosed = errfmt.Errorf("shockwave subscriber is closed")
)

var (
	_ CallbackSubscriber = (*ShockwaveSubscriber)(nil)
	_ Subscriber         = (*ShockwaveSubscriber)(nil)
)

// ShockwaveSubscriberOption defines functional options for configuring a ShockwaveSubscriber.
type ShockwaveSubscriberOption func(*ShockwaveSubscriber)

// WithAsyncDispatch configures whether the subscriber broadcasts shockwaves asynchronously.
func WithAsyncDispatch(async bool) ShockwaveSubscriberOption {
	return func(s *ShockwaveSubscriber) {
		s.async = async
	}
}

// WithBufferSize configures the channel capacity for asynchronous shockwave dispatching.
func WithBufferSize(size int) ShockwaveSubscriberOption {
	return func(s *ShockwaveSubscriber) {
		if size > 0 {
			s.bufferSize = size
		}
	}
}

// WithLogger configures a structured logger for telemetry and overflow warnings.
func WithLogger(l logging.Logger) ShockwaveSubscriberOption {
	return func(s *ShockwaveSubscriber) {
		s.logger = l
	}
}

// WithName overrides the default subscriber identifier name.
func WithName(name string) ShockwaveSubscriberOption {
	return func(s *ShockwaveSubscriber) {
		if strings.TrimSpace(name) != emptyValue {
			s.name = strings.TrimSpace(name)
		}
	}
}

// ShockwaveSubscriber emits reactive invalidation shockwaves across the InvalidationShockwaveBus
// when scheduler and lifecycle callback payloads indicate mutation, waking up storage listeners
// without polling overhead.
type ShockwaveSubscriber struct {
	name       string
	bus        *storage.InvalidationShockwaveBus
	async      bool
	bufferSize int
	logger     logging.Logger

	eventCh  chan storage.MutationEvent
	ctx      context.Context
	cancel   context.CancelFunc
	workerWg sync.WaitGroup
	closed   atomic.Bool
	mu       sync.RWMutex
}

// NewShockwaveSubscriber creates a subscriber linked to an invalidation bus with optional settings.
func NewShockwaveSubscriber(bus *storage.InvalidationShockwaveBus, opts ...ShockwaveSubscriberOption) *ShockwaveSubscriber {
	if bus == nil {
		bus = storage.GetGlobalInvalidationBus()
	}

	sub := &ShockwaveSubscriber{
		name:       SubscriberNameShockwave,
		bus:        bus,
		bufferSize: DefaultShockwaveBufferSize,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(sub)
		}
	}

	if sub.async {
		sub.eventCh = make(chan storage.MutationEvent, sub.bufferSize)
		sub.ctx, sub.cancel = context.WithCancel(context.Background())
		sub.workerWg.Add(1)
		goroutinelabels.NewGoroutine("shockwave_subscriber_worker", "async invalidation shockwave worker").StartSimple(sub.runWorker)
	}

	return sub
}

// NewAsyncShockwaveSubscriber creates an asynchronous ShockwaveSubscriber with the specified buffer capacity.
func NewAsyncShockwaveSubscriber(bus *storage.InvalidationShockwaveBus, bufferSize int, opts ...ShockwaveSubscriberOption) *ShockwaveSubscriber {
	allOpts := make([]ShockwaveSubscriberOption, 0, len(opts)+2)
	allOpts = append(allOpts, WithAsyncDispatch(true), WithBufferSize(bufferSize))
	allOpts = append(allOpts, opts...)
	return NewShockwaveSubscriber(bus, allOpts...)
}

// Name returns the identifier of the shockwave subscriber.
func (s *ShockwaveSubscriber) Name() string {
	return s.name
}

// Bus returns the underlying InvalidationShockwaveBus instance.
func (s *ShockwaveSubscriber) Bus() *storage.InvalidationShockwaveBus {
	return s.bus
}

// IsAsync returns whether asynchronous dispatch mode is enabled.
func (s *ShockwaveSubscriber) IsAsync() bool {
	return s.async
}

// IsClosed returns whether the subscriber has been terminated.
func (s *ShockwaveSubscriber) IsClosed() bool {
	return s.closed.Load()
}

// Notify broadcasts a storage MutationEvent shockwave across the invalidation bus.
func (s *ShockwaveSubscriber) Notify(ctx context.Context, entry *CallbackEntry) error {
	if s == nil {
		return errfmt.Errorf("shockwave subscriber is nil")
	}
	if s.closed.Load() {
		return ErrSubscriberClosed
	}
	if entry == nil {
		return ErrNilCallbackEntry
	}
	if entry.Payload == nil {
		return ErrNilPayload
	}
	if s.bus == nil {
		return ErrNilBus
	}

	if ctx == nil {
		ctx = context.Background()
	}

	event := s.buildMutationEvent(entry)

	if s.async {
		select {
		case s.eventCh <- event:
			return nil
		default:
			if s.logger != nil {
				s.logger.Warn("shockwave subscriber queue full, dropping event",
					logging.String("kind", event.Kind),
					logging.String("id", event.ID))
			}
			return nil
		}
	}

	s.bus.Broadcast(ctx, event)
	return nil
}

func (s *ShockwaveSubscriber) buildMutationEvent(entry *CallbackEntry) storage.MutationEvent {
	payload := entry.Payload

	objID, _ := payload[FieldKeyObjectID].(string)
	if objID == emptyValue {
		objID, _ = payload[FieldKeyID].(string)
	}
	if objID == emptyValue {
		objID, _ = payload[FieldKeyJobID].(string)
	}

	kind, _ := payload[objects.FieldKeyKind].(string)
	kind = strings.TrimSpace(kind)
	if kind == emptyValue && objID != emptyValue {
		kind = storage.InferKindFromID(objID)
	}
	if kind == emptyValue {
		if _, hasJobID := payload[FieldKeyJobID]; hasJobID || strings.HasPrefix(objID, "job-") {
			kind = DefaultKindSchedulerJob
		} else {
			kind = DefaultKindUnknown
		}
	}

	opStr, _ := payload[FieldKeyOp].(string)
	var op storage.MutationOp
	switch strings.ToLower(strings.TrimSpace(opStr)) {
	case string(storage.MutationOpDelete):
		op = storage.MutationOpDelete
	default:
		op = storage.MutationOpPut
	}

	ts := entry.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	return storage.MutationEvent{
		Op:         op,
		Kind:       kind,
		ID:         objID,
		Timestamp:  ts,
		ObjectData: payload,
	}
}

func (s *ShockwaveSubscriber) runWorker() {
	defer s.workerWg.Done()
	for {
		select {
		case <-s.ctx.Done():
			for {
				select {
				case ev := <-s.eventCh:
					s.bus.Broadcast(context.Background(), ev)
				default:
					return
				}
			}
		case ev, ok := <-s.eventCh:
			if !ok {
				return
			}
			s.bus.Broadcast(s.ctx, ev)
		}
	}
}

// Close gracefully terminates any async worker goroutines and drains the event channel.
func (s *ShockwaveSubscriber) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	if s.async {
		if s.cancel != nil {
			s.cancel()
		}
		s.workerWg.Wait()
	}
	return nil
}
