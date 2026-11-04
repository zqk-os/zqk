package ambience

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
)

// mockFailingMesh simulates failures during mesh interactions.
type mockFailingMesh struct {
	subscribeErr error
	publishErr   error
}

func (m *mockFailingMesh) Publish(ctx context.Context, event AmbientEvent) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	return nil
}

func (m *mockFailingMesh) Subscribe(ctx context.Context, types []EventType) (<-chan AmbientEvent, error) {
	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}
	ch := make(chan AmbientEvent, 1)
	return ch, nil
}

func TestSignalAggregator_LifecycleAndErrorBranches(t *testing.T) {
	t.Parallel()

	// 1. Test Subscribe error on Start
	expectedErr := errors.New("failed to subscribe to mesh")
	failingMesh := &mockFailingMesh{subscribeErr: expectedErr}
	agg := NewSignalAggregator(failingMesh, 50*time.Millisecond, 5)

	err := agg.Start(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected subscribe error %v, got %v", expectedErr, err)
	}

	// 2. Test already running idempotency on Start
	mesh := NewInMemoryEventMesh()
	agg2 := NewSignalAggregator(mesh, 50*time.Millisecond, 5)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := agg2.Start(ctx); err != nil {
		t.Fatalf("unexpected Start error: %v", err)
	}
	// Calling Start again while running should return nil
	if err := agg2.Start(ctx); err != nil {
		t.Fatalf("expected nil on second Start, got %v", err)
	}

	// 3. Test Stop idempotency
	if err := agg2.Stop(); err != nil {
		t.Fatalf("unexpected Stop error: %v", err)
	}
	if err := agg2.Stop(); err != nil {
		t.Fatalf("expected nil on second Stop, got %v", err)
	}
}

func TestSignalAggregator_EmptyTickerFlush(t *testing.T) {
	t.Parallel()

	mesh := NewInMemoryEventMesh()
	// Short window so ticker fires quickly without any events
	agg := NewSignalAggregator(mesh, 10*time.Millisecond, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := agg.Start(ctx); err != nil {
		t.Fatalf("failed to start aggregator: %v", err)
	}

	// Let ticker tick at least twice while s.events is empty
	time.Sleep(35 * time.Millisecond)

	if err := agg.Stop(); err != nil {
		t.Fatalf("failed to stop aggregator: %v", err)
	}
}

func TestSignalAggregator_FlushChannelOverflowDefault(t *testing.T) {
	t.Parallel()

	mesh := NewInMemoryEventMesh()
	agg := NewSignalAggregator(mesh, time.Hour, 10)

	// flushCh capacity is 10. Sending 12 items will trigger the default: branch.
	for i := 0; i < 12; i++ {
		agg.flush([]AmbientEvent{{ID: "ev"}})
	}

	// Aggregated() returns channel
	ch := agg.Aggregated()
	if ch == nil {
		t.Fatal("expected non-nil aggregated channel")
	}

	// Drain flushed items
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto drained
		}
	}
drained:
	if count != 10 {
		t.Fatalf("expected exactly 10 buffered items before overflow drop, got %d", count)
	}
}

func TestFSEventsEngine_LifecycleAndErrorBranches(t *testing.T) {
	t.Parallel()

	// 1. Test Subscribe error on Start
	expectedErr := errors.New("failed to subscribe to fsevents")
	failingMesh := &mockFailingMesh{subscribeErr: expectedErr}
	engine := NewFSEventsEngine(failingMesh)

	err := engine.Start(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected subscribe error %v, got %v", expectedErr, err)
	}

	// 2. Stop when not running
	if err := engine.Stop(); err != nil {
		t.Fatalf("expected nil on Stop when not running, got %v", err)
	}
}

func TestInMemoryEventMesh_BlockedChannelContextCancellation(t *testing.T) {
	t.Parallel()

	mesh := NewInMemoryEventMesh()
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()

	// Subscribe creates channel with buffer capacity 100
	_, err := mesh.Subscribe(subCtx, []EventType{EventFileModified})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Fill subscriber buffer (100 items)
	pubCtx := context.Background()
	for i := 0; i < 100; i++ {
		if err := mesh.Publish(pubCtx, AmbientEvent{Type: EventFileModified, URI: "a.go"}); err != nil {
			t.Fatalf("fill buffer failed at %d: %v", i, err)
		}
	}

	// Now subscriber buffer is full. Publishing with a short timeout context will block and time out.
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer timeoutCancel()

	err = mesh.Publish(timeoutCtx, AmbientEvent{Type: EventFileModified, URI: "blocked.go"})
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context deadline or canceled error, got %v", err)
	}
}

func TestInMemoryEventMesh_EventSourcingFullBranches(t *testing.T) {
	mesh := NewInMemoryEventMesh()
	tempDir := t.TempDir()
	secCtx := pkgctx.NewSystemSecurityContext()

	mesh.EnableEventSourcing(tempDir, secCtx)

	ctx := context.Background()

	// 1. DocEntry via .md with JSON payload
	ev1 := AmbientEvent{
		ID:        "ev-md",
		Type:      EventFileModified,
		URI:       "docs/guide.md",
		Timestamp: time.Now().Unix(),
		Payload:   []byte(`{"status":"updated","lines":42}`),
	}
	if err := mesh.Publish(ctx, ev1); err != nil {
		t.Fatalf("Publish ev1 failed: %v", err)
	}

	// 2. TechnicalSpec via .yaml with non-JSON payload
	ev2 := AmbientEvent{
		ID:        "ev-yaml",
		Type:      EventFileModified,
		URI:       "configs/service.yaml",
		Timestamp: time.Now().Unix(),
		Payload:   []byte("spec_key: value_raw"),
	}
	if err := mesh.Publish(ctx, ev2); err != nil {
		t.Fatalf("Publish ev2 failed: %v", err)
	}

	// 3. TechnicalSpec via .json with empty payload
	ev3 := AmbientEvent{
		ID:        "ev-json",
		Type:      EventFileModified,
		URI:       "schema/meta.json",
		Timestamp: time.Now().Unix(),
		Payload:   nil,
	}
	if err := mesh.Publish(ctx, ev3); err != nil {
		t.Fatalf("Publish ev3 failed: %v", err)
	}

	// 4. KindAuditEvent via other URI suffix (e.g. .go) with string payload
	ev4 := AmbientEvent{
		ID:        "ev-go",
		Type:      EventFileModified,
		URI:       "pkg/main.go",
		Timestamp: time.Now().Unix(),
		Payload:   []byte("raw code change"),
	}
	if err := mesh.Publish(ctx, ev4); err != nil {
		t.Fatalf("Publish ev4 failed: %v", err)
	}

	// 5. Empty URI with JSON payload
	ev5 := AmbientEvent{
		ID:        "ev-empty-uri",
		Type:      EventFocusChanged,
		URI:       "",
		Timestamp: time.Now().Unix(),
		Payload:   []byte(`{"focus":"editor"}`),
	}
	if err := mesh.Publish(ctx, ev5); err != nil {
		t.Fatalf("Publish ev5 failed: %v", err)
	}

	// 6. No subscribers for type (subscribers[event.Type] not found branch)
	ev6 := AmbientEvent{
		ID:        "ev-unsubscribed",
		Type:      EventTestFailed,
		URI:       "tests/fail.log",
		Timestamp: time.Now().Unix(),
		Payload:   []byte(`{"exit_code":1}`),
	}
	if err := mesh.Publish(ctx, ev6); err != nil {
		t.Fatalf("Publish ev6 failed: %v", err)
	}

	// Deterministically wait for asynchronous audit event emissions
	mesh.WaitForAudits()
	storage.ScrubProjectRootForTempCleanup(tempDir, 20, 25*time.Millisecond)
}
