package callback

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestMultiSubscriberDispatcher_ConcurrentRouting(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("system")
	d := NewMultiSubscriberDispatcher(logger)

	var count int32
	var wg sync.WaitGroup
	wg.Add(3)

	for i := 0; i < 3; i++ {
		name := string(rune('A' + i))
		d.Register(NewFuncSubscriber(name, func(ctx context.Context, entry *CallbackEntry) error {
			atomic.AddInt32(&count, 1)
			wg.Done()
			return nil
		}))
	}

	entry := &CallbackEntry{
		Payload:   map[string]any{"job_id": "test-job-1"},
		Timestamp: time.Now().UTC(),
	}

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}

	wg.Wait()
	if atomic.LoadInt32(&count) != 3 {
		t.Fatalf("expected 3 subscriber invocations, got %d", count)
	}
}

func TestMultiSubscriberDispatcher_ErrorIsolation(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("system")
	d := NewMultiSubscriberDispatcher(logger)

	var successCalled int32
	d.Register(NewFuncSubscriber("failing", func(ctx context.Context, entry *CallbackEntry) error {
		return errfmt.Errorf("simulated subscriber failure")
	}))
	d.Register(NewFuncSubscriber("healthy", func(ctx context.Context, entry *CallbackEntry) error {
		atomic.AddInt32(&successCalled, 1)
		return nil
	}))

	entry := &CallbackEntry{
		Payload:   map[string]any{"job_id": "job-iso"},
		Timestamp: time.Now().UTC(),
	}

	err := d.Dispatch(context.Background(), entry)
	if err == nil {
		t.Fatalf("expected error from failing subscriber")
	}

	if atomic.LoadInt32(&successCalled) != 1 {
		t.Fatalf("expected healthy subscriber to still execute, got %d", successCalled)
	}
}

func TestMultiSubscriberDispatcher_Unregister(t *testing.T) {
	t.Parallel()
	d := NewMultiSubscriberDispatcher(nil)
	d.Register(NewFuncSubscriber("sub1", nil))
	d.Register(NewFuncSubscriber("sub2", nil))

	if len(d.Subscribers()) != 2 {
		t.Fatalf("expected 2 subscribers, got %d", len(d.Subscribers()))
	}

	d.Unregister("sub1")
	subs := d.Subscribers()
	if len(subs) != 1 || subs[0].Name() != "sub2" {
		t.Fatalf("expected only sub2 to remain, got %v", subs)
	}

	// Unregister non-existent should not panic
	d.Unregister("non-existent")
}

func TestKernelWALSubscriber_EventSynthesis(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	sub := NewKernelWALSubscriber(tempDir)

	entry := &CallbackEntry{
		Payload: map[string]any{
			"job_id":                     "job-wal-1",
			objects.FieldKeyCallbackType: callbackTypeCompletion,
			"object_id":                  "BLI-TEST-001",
			objects.FieldKeyKind:         "backlog_item",
			"status":                     "complete",
			"from_status":                "testing",
			"success":                    true,
		},
		Timestamp: time.Now().UTC(),
	}

	if err := sub.Notify(context.Background(), entry); err != nil {
		t.Fatalf("failed to notify WAL subscriber: %v", err)
	}

	// Verify WAL contains the synthesized event
	wal, err := lifecycle.NewLifecycleEventWAL(tempDir)
	if err != nil {
		t.Fatalf("failed to open WAL: %v", err)
	}
	var events []*lifecycle.LifecycleEvent
	err = wal.ReplayFrom(0, func(ev *lifecycle.LifecycleEvent) error {
		events = append(events, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("failed to replay WAL: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event in WAL, got %d", len(events))
	}
	ev := events[0]
	if ev.ID != "BLI-TEST-001" || ev.ToStatus != "complete" || ev.EventType != lifecycle.EventTypeStatusTransition {
		t.Fatalf("unexpected synthesized event: %+v", ev)
	}
}

func TestShockwaveSubscriber_Broadcast(t *testing.T) {
	t.Parallel()
	bus := storage.NewInvalidationShockwaveBus()
	sub := NewShockwaveSubscriber(bus)

	var received storage.MutationEvent
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(storage.InvalidationSubscriberFunc(func(ctx context.Context, event storage.MutationEvent) error {
		received = event
		wg.Done()
		return nil
	}))

	entry := &CallbackEntry{
		Payload: map[string]any{
			"job_id":             "job-shock-1",
			"object_id":          "ATK-TEST-001",
			objects.FieldKeyKind: "agent_task",
		},
		Timestamp: time.Now().UTC(),
	}

	if err := sub.Notify(context.Background(), entry); err != nil {
		t.Fatalf("failed to notify shockwave subscriber: %v", err)
	}

	wg.Wait()
	if received.ID != "ATK-TEST-001" || received.Kind != "agent_task" {
		t.Fatalf("unexpected mutation event received: %+v", received)
	}
}

func TestProcessor_CustomSubscriberIntegration(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "custom.log")

	if err := p.Initialize(tempDir, logFile, true, 10, &TimestampSorter{}, "text"); err != nil {
		t.Fatalf("failed to init processor: %v", err)
	}
	defer p.Shutdown()

	var customNotified int32
	p.RegisterSubscriber(NewFuncSubscriber("custom_sub", func(ctx context.Context, entry *CallbackEntry) error {
		atomic.AddInt32(&customNotified, 1)
		return nil
	}))

	payload := map[string]any{
		"job_id":  "custom-job",
		"success": true,
	}

	if err := p.ProcessDirect(tempDir, logFile, true, payload, "text"); err != nil {
		t.Fatalf("failed to process direct: %v", err)
	}

	if atomic.LoadInt32(&customNotified) != 1 {
		t.Fatalf("expected custom subscriber to be notified, got %d", customNotified)
	}
}
