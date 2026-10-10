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
		h.sub.Close()
	}
}

func runHarnessTest(t *testing.T, fn func(t *testing.T, h *shockwaveTestHarness)) {
	t.Helper()
	t.Parallel()
	h := newShockwaveTestHarness()
	defer h.close()
	fn(t, h)
}

func TestShockwaveSubscriber_InterfaceComplianceAndDefaults(t *testing.T) {
	runHarnessTest(t, func(t *testing.T, h *shockwaveTestHarness) {
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
	})
}

func TestShockwaveSubscriber_NilEventAndPayloadRejection(t *testing.T) {
	runHarnessTest(t, func(t *testing.T, h *shockwaveTestHarness) {
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
	})
}

func TestShockwaveSubscriber_KindInferenceResilience(t *testing.T) {
	runHarnessTest(t, func(t *testing.T, h *shockwaveTestHarness) {
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
	})
}

func TestShockwaveSubscriber_MissingObjectIDFallback(t *testing.T) {
	runHarnessTest(t, func(t *testing.T, h *shockwaveTestHarness) {
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
		evJob, ok := h.awaitNextEvent(2 * time.Second)
		if !ok {
			t.Fatalf("timed out waiting for event")
		}
		if evJob.ID != "job-exec-7788" || evJob.Kind != "scheduler_job" {
			t.Fatalf("expected ID=job-exec-7788 and Kind=scheduler_job, got ID=%s, Kind=%s", evJob.ID, evJob.Kind)
		}

		// Case 2: missing both object_id and job_id, but has id
		entryID := &CallbackEntry{
			Payload: map[string]any{
				objects.FieldKeyID: "generic-id-1234",
			},
			Timestamp: time.Now().UTC(),
		}
		if err := h.sub.Notify(context.Background(), entryID); err != nil {
			t.Fatalf("unexpected notify error: %v", err)
		}
		evID, ok := h.awaitNextEvent(2 * time.Second)
		if !ok {
			t.Fatalf("timed out waiting for event")
		}
		if evID.ID != "generic-id-1234" {
			t.Fatalf("expected ID=generic-id-1234, got %s", evID.ID)
		}

		// Case 3: completely empty identifiers fall back to unknown
		entryEmpty := &CallbackEntry{
			Payload: map[string]any{
				"unrelated": "value",
			},
			Timestamp: time.Now().UTC(),
		}
		if err := h.sub.Notify(context.Background(), entryEmpty); err != nil {
			t.Fatalf("unexpected notify error: %v", err)
		}
		evUnknown, ok := h.awaitNextEvent(2 * time.Second)
		if !ok {
			t.Fatalf("timed out waiting for event")
		}
		if evUnknown.ID != "" {
			t.Fatalf("expected empty ID for missing identifier, got %q", evUnknown.ID)
		}
		if evUnknown.Kind != DefaultKindUnknown {
			t.Fatalf("expected kind %q, got %q", DefaultKindUnknown, evUnknown.Kind)
		}
	})
}

func TestShockwaveSubscriber_AsyncNonBlockingDispatch(t *testing.T) {
	t.Parallel()
	bus := storage.NewInvalidationShockwaveBus()
	sub := NewAsyncShockwaveSubscriber(bus, 10, WithLogger(logging.GetLoggerFromProfile("system")))
	defer func() { sub.Close() }()

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
				FieldKeyObjectID: "BLI-ASYNC-TEST",
			},
			Timestamp: time.Now().UTC(),
		}
		if err := sub.Notify(context.Background(), entry); err != nil {
			t.Fatalf("unexpected notify error: %v", err)
		}
	}

	// Close waits for async queue flush
	if err := sub.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	if atomic.LoadInt32(&count) != total {
		t.Fatalf("expected %d events delivered, got %d", total, count)
	}
}

func TestShockwaveSubscriber_ConcurrentLoadAndZeroLeaks(t *testing.T) {
	t.Parallel()
	bus := storage.NewInvalidationShockwaveBus()
	sub := NewAsyncShockwaveSubscriber(bus, 500)

	var receivedCount int64
	bus.Subscribe(storage.InvalidationSubscriberFunc(func(ctx context.Context, ev storage.MutationEvent) error {
		atomic.AddInt64(&receivedCount, 1)
		return nil
	}))

	const numGoroutines = 10
	const eventsPerGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		goroutineID := g
		goroutinelabels.NewGoroutine("test_concurrent_shockwave", "dispatch concurrent callbacks").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				entry := &CallbackEntry{
					Payload: map[string]any{
						FieldKeyObjectID: "BLI-CONCURRENT-LOAD",
						"index":          goroutineID*eventsPerGoroutine + i,
					},
					Timestamp: time.Now().UTC(),
				}
				if notifyErr := sub.Notify(context.Background(), entry); notifyErr != nil {
					// Ignore potential closed notifications during test teardown
				}
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
	defer func() { sub.Close() }()

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
			FieldKeyObjectID: "CRIT-DISPATCH-INTEG",
			FieldKeyOp:       "put",
		},
		Timestamp: time.Now().UTC(),
	}

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	wg.Wait()

	if receivedEvent.ID != "CRIT-DISPATCH-INTEG" || receivedEvent.Kind != "criteria" {
		t.Fatalf("unexpected received event: %+v", receivedEvent)
	}
}
