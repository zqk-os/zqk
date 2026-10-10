package callback

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type shockwaveTestHarness struct {
	bus             *storage.InvalidationShockwaveBus
	sub             *ShockwaveSubscriber
	receivedMutMu   sync.Mutex
	receivedEvents  []storage.MutationEvent
	eventNotifyChan chan storage.MutationEvent
}

func newShockwaveTestHarness(opts ...ShockwaveSubscriberOption) *shockwaveTestHarness {
	bus := storage.NewInvalidationShockwaveBus()
	sub := NewShockwaveSubscriber(bus, opts...)
	harness := &shockwaveTestHarness{
		bus:             bus,
		sub:             sub,
		receivedEvents:  make([]storage.MutationEvent, 0),
		eventNotifyChan: make(chan storage.MutationEvent, 1000),
	}

	bus.Subscribe(storage.InvalidationSubscriberFunc(func(ctx context.Context, ev storage.MutationEvent) error {
		harness.receivedMutMu.Lock()
		harness.receivedEvents = append(harness.receivedEvents, ev)
		harness.receivedMutMu.Unlock()

		select {
		case harness.eventNotifyChan <- ev:
		default:
		}
		return nil
	}))

	return harness
}

func (h *shockwaveTestHarness) awaitNextEvent(timeout time.Duration) (storage.MutationEvent, bool) {
	select {
	case ev := <-harnessEventWait(h.eventNotifyChan):
		return ev, true
	case <-time.After(timeout):
		return storage.MutationEvent{}, false
	}
}

func harnessEventWait(ch <-chan storage.MutationEvent) <-chan storage.MutationEvent {
	return ch
}

func (h *shockwaveTestHarness) eventCount() int {
	h.receivedMutMu.Lock()
	defer h.receivedMutMu.Unlock()
	return len(h.receivedEvents)
}

func (h *shockwaveTestHarness) close() {
	if h.sub != nil {
		_ = h.sub.Close()
	}
}

func TestShockwaveSubscriber_InterfaceComplianceAndDefaults(t *testing.T) {
	t.Parallel()
	h := newShockwaveTestHarness()
	defer h.close()

	var cbSub CallbackSubscriber = h.sub
	if cbSub.Name() != SubscriberNameShockwave {
		t.Fatalf("expected subscriber name %q, got %q", SubscriberNameShockwave, cbSub.Name())
	}
	if h.sub.IsAsync() {
		t.Fatalf("expected default subscriber to be synchronous")
	}
	if h.sub.IsClosed() {
		t.Fatalf("expected active subscriber not to be closed")
	}
	if h.sub.Bus() != h.bus {
		t.Fatalf("expected bus pointer to match harness bus")
	}
}

func TestShockwaveSubscriber_NilEventAndPayloadRejection(t *testing.T) {
	t.Parallel()
	h := newShockwaveTestHarness()
	defer h.close()

	ctx := context.Background()

	// 1. Nil entry rejection
	if err := h.sub.Notify(ctx, nil); !errors.Is(err, ErrNilCallbackEntry) {
		t.Fatalf("expected ErrNilCallbackEntry on nil entry, got %v", err)
	}

	// 2. Nil payload rejection
	entryNilPayload := &CallbackEntry{Timestamp: time.Now().UTC()}
	if err := h.sub.Notify(ctx, entryNilPayload); !errors.Is(err, ErrNilPayload) {
		t.Fatalf("expected ErrNilPayload on entry with nil payload, got %v", err)
	}

	// 3. Nil subscriber invocation
	var nilSub *ShockwaveSubscriber
	if err := nilSub.Notify(ctx, entryNilPayload); err == nil {
		t.Fatalf("expected error when notifying nil subscriber")
	}
}

func TestShockwaveSubscriber_KindInferenceResilience(t *testing.T) {
	t.Parallel()
	h := newShockwaveTestHarness()
	defer h.close()

	cases := []struct {
		desc     string
		payload  map[string]any
		wantKind string
		wantID   string
		wantOp   storage.MutationOp
	}{
		{
			desc: "Infers backlog_item from BLI prefix",
			payload: map[string]any{
				FieldKeyObjectID: "BLI-1791620297103501000-92641179",
			},
			wantKind: "backlog_item",
			wantID:   "BLI-1791620297103501000-92641179",
			wantOp:   storage.MutationOpPut,
		},
		{
			desc: "Infers requirement from REQ prefix",
			payload: map[string]any{
				FieldKeyObjectID: "REQ-1791620296568001000-5ab62022",
				FieldKeyOp:       "delete",
			},
			wantKind: "requirement",
			wantID:   "REQ-1791620296568001000-5ab62022",
			wantOp:   storage.MutationOpDelete,
		},
		{
			desc: "Infers criteria from CRIT prefix",
			payload: map[string]any{
				FieldKeyObjectID: "CRIT-1791620297103501000-9fa62e60",
			},
			wantKind: "criteria",
			wantID:   "CRIT-1791620297103501000-9fa62e60",
			wantOp:   storage.MutationOpPut,
		},
		{
			desc: "Explicit kind is respected over prefix",
			payload: map[string]any{
				FieldKeyObjectID:     "CUSTOM-001",
				objects.FieldKeyKind: "agent_task",
			},
			wantKind: "agent_task",
			wantID:   "CUSTOM-001",
			wantOp:   storage.MutationOpPut,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			entry := &CallbackEntry{
				Payload:   tc.payload,
				Timestamp: time.Now().UTC(),
			}
			if err := h.sub.Notify(context.Background(), entry); err != nil {
				t.Fatalf("unexpected notify error: %v", err)
			}
			ev, ok := h.awaitNextEvent(2 * time.Second)
			if !ok {
				t.Fatalf("timed out waiting for event")
			}
			if ev.Kind != tc.wantKind || ev.ID != tc.wantID || ev.Op != tc.wantOp {
				t.Fatalf("mismatched event: got Kind=%q, ID=%q, Op=%q; want Kind=%q, ID=%q, Op=%q",
					ev.Kind, ev.ID, ev.Op, tc.wantKind, tc.wantID, tc.wantOp)
			}
		})
	}
}

func TestShockwaveSubscriber_MissingObjectIDFallback(t *testing.T) {
	t.Parallel()
	h := newShockwaveTestHarness()
	defer h.close()

	// Case 1: missing object_id but has job_id
	entryJob := &CallbackEntry{
		Payload: map[string]any{
			FieldKeyJobID: "job-exec-7788",
		},
		Timestamp: time.Now().UTC(),
	}
	if err := h.sub.Notify(context.Background(), entryJob); err != nil {
		t.Fatalf("unexpected notify error: %v", err)
	}
	ev, ok := h.awaitNextEvent(2 * time.Second)
	if !ok {
		t.Fatalf("timed out waiting for job fallback event")
	}
	if ev.ID != "job-exec-7788" || ev.Kind != DefaultKindSchedulerJob {
		t.Fatalf("unexpected fallback: ID=%q, Kind=%q", ev.ID, ev.Kind)
	}

	// Case 2: completely empty keys fall back to unknown
	entryUnknown := &CallbackEntry{
		Payload: map[string]any{
			"unrelated_field": "test",
		},
		Timestamp: time.Now().UTC(),
	}
	if err := h.sub.Notify(context.Background(), entryUnknown); err != nil {
		t.Fatalf("unexpected notify error: %v", err)
	}
	evUnknown, ok := h.awaitNextEvent(2 * time.Second)
	if !ok {
		t.Fatalf("timed out waiting for unknown fallback event")
	}
	if evUnknown.Kind != DefaultKindUnknown {
		t.Fatalf("expected kind %q, got %q", DefaultKindUnknown, evUnknown.Kind)
	}
}

func TestShockwaveSubscriber_AsyncNonBlockingDispatch(t *testing.T) {
	t.Parallel()
	bus := storage.NewInvalidationShockwaveBus()
	sub := NewAsyncShockwaveSubscriber(bus, 10, WithLogger(logging.GetLoggerFromProfile("system")))
	defer func() { _ = sub.Close() }()

	if !sub.IsAsync() {
		t.Fatalf("expected subscriber to be configured async")
	}

	var count int32
	bus.Subscribe(storage.InvalidationSubscriberFunc(func(ctx context.Context, ev storage.MutationEvent) error {
		atomic.AddInt32(&count, 1)
		return nil
	}))

	const total = 5
	for i := 0; i < total; i++ {
		entry := &CallbackEntry{
			Payload: map[string]any{
				FieldKeyObjectID: "BLI-ASYNC-001",
			},
			Timestamp: time.Now().UTC(),
		}
		if err := sub.Notify(context.Background(), entry); err != nil {
			t.Fatalf("unexpected async notify error: %v", err)
		}
	}

	// Wait for async drainage
	if err := sub.Close(); err != nil {
		t.Fatalf("failed to close async subscriber: %v", err)
	}

	if atomic.LoadInt32(&count) != total {
		t.Fatalf("expected %d events delivered, got %d", total, count)
	}

	// Rejection after close
	entryAfterClose := &CallbackEntry{
		Payload: map[string]any{FieldKeyObjectID: "BLI-ASYNC-002"},
	}
	if err := sub.Notify(context.Background(), entryAfterClose); !errors.Is(err, ErrSubscriberClosed) {
		t.Fatalf("expected ErrSubscriberClosed after close, got %v", err)
	}
}

func TestShockwaveSubscriber_ConcurrentLoadAndZeroLeaks(t *testing.T) {
	t.Parallel()
	bus := storage.NewInvalidationShockwaveBus()
	sub := NewAsyncShockwaveSubscriber(bus, 1000)

	var receivedCount int64
	bus.Subscribe(storage.InvalidationSubscriberFunc(func(ctx context.Context, ev storage.MutationEvent) error {
		atomic.AddInt64(&receivedCount, 1)
		return nil
	}))

	const numGoroutines = 50
	const eventsPerGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		goroutineID := i
		goroutinelabels.NewGoroutine("test_concurrent_load", "firing concurrent callbacks").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				entry := &CallbackEntry{
					Payload: map[string]any{
						FieldKeyObjectID:     "BLI-CONCURR-001",
						objects.FieldKeyKind: "backlog_item",
						"goroutine_id":       goroutineID,
						"seq":                j,
					},
					Timestamp: time.Now().UTC(),
				}
				_ = sub.Notify(context.Background(), entry)
			}
		})
	}

	wg.Wait()

	if err := sub.Close(); err != nil {
		t.Fatalf("error closing concurrent subscriber: %v", err)
	}

	expectedTotal := int64(numGoroutines * eventsPerGoroutine)
	if atomic.LoadInt64(&receivedCount) != expectedTotal {
		t.Fatalf("expected %d events dispatched, got %d", expectedTotal, receivedCount)
	}
}

func TestShockwaveSubscriber_DispatcherIntegration(t *testing.T) {
	t.Parallel()
	bus := storage.NewInvalidationShockwaveBus()
	sub := NewShockwaveSubscriber(bus)
	defer func() { _ = sub.Close() }()

	d := NewMultiSubscriberDispatcher(logging.GetLoggerFromProfile("system"))
	d.Register(sub)

	var receivedEvent storage.MutationEvent
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(storage.InvalidationSubscriberFunc(func(ctx context.Context, ev storage.MutationEvent) error {
		receivedEvent = ev
		wg.Done()
		return nil
	}))

	entry := &CallbackEntry{
		Payload: map[string]any{
			FieldKeyJobID:        "job-dispatcher-01",
			FieldKeyObjectID:     "BLI-DISP-001",
			objects.FieldKeyKind: "backlog_item",
		},
		Timestamp: time.Now().UTC(),
	}

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}

	wg.Wait()
	if receivedEvent.ID != "BLI-DISP-001" || receivedEvent.Kind != "backlog_item" {
		t.Fatalf("unexpected received mutation event: %+v", receivedEvent)
	}
}
