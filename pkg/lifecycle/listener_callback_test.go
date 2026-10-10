package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type testCallbackWaker struct {
	name        string
	mu          sync.Mutex
	events      []*LifecycleEvent
	errToReturn error
}

func newTestCallbackWaker(name string) *testCallbackWaker {
	return &testCallbackWaker{name: name}
}

func (w *testCallbackWaker) Name() string {
	return w.name
}

func (w *testCallbackWaker) Wake(_ context.Context, ev *LifecycleEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, ev)
	return w.errToReturn
}

func (w *testCallbackWaker) ReceivedEvents() []*LifecycleEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	res := make([]*LifecycleEvent, len(w.events))
	copy(res, w.events)
	return res
}

func TestListenerCallbackWakerRegistration(t *testing.T) {
	tempDir := t.TempDir()
	wal, err := NewLifecycleEventWAL(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	ch := make(chan TransitionRequest, 10)
	listener := NewListener(tempDir, wal, nil, ch, nil)

	assert.Empty(t, listener.Wakers())

	w1 := newTestCallbackWaker("waker-1")
	w2 := newTestCallbackWaker("waker-2")

	listener.RegisterWaker(w1)
	listener.RegisterWaker(w2)
	assert.Len(t, listener.Wakers(), 2)

	// Idempotent / overwrite registration
	listener.RegisterWaker(w1)
	assert.Len(t, listener.Wakers(), 2)

	// Unregister
	listener.UnregisterWaker("waker-1")
	assert.Len(t, listener.Wakers(), 1)
	assert.Equal(t, "waker-2", listener.Wakers()[0].Name())

	// Unregister non-existent
	listener.UnregisterWaker("non-existent")
	assert.Len(t, listener.Wakers(), 1)
}

func TestListenerDispatchCallbackNow(t *testing.T) {
	tempDir := t.TempDir()
	wal, err := NewLifecycleEventWAL(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	ch := make(chan TransitionRequest, 10)
	listener := NewListener(tempDir, wal, nil, ch, nil)

	w1 := newTestCallbackWaker("w1")
	w2 := newTestCallbackWaker("w2")
	w2.errToReturn = errors.New("simulated waker error")

	listener.RegisterWaker(w1)
	listener.RegisterWaker(w2)

	ctx := context.Background()
	ev := &LifecycleEvent{
		EventType: EventTypeSchedulerCallback,
		Kind:      "job",
		ID:        "job-12345",
	}

	errs := listener.DispatchCallbackNow(ctx, ev)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "simulated waker error")

	assert.Len(t, w1.ReceivedEvents(), 1)
	assert.Equal(t, "job-12345", w1.ReceivedEvents()[0].ID)

	assert.Len(t, w2.ReceivedEvents(), 1)
	assert.Equal(t, "job-12345", w2.ReceivedEvents()[0].ID)
}

func TestListenerProcessEventSchedulerCallback(t *testing.T) {
	tempDir := t.TempDir()
	wal, err := NewLifecycleEventWAL(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	ch := make(chan TransitionRequest, 10)
	listener := NewListener(tempDir, wal, nil, ch, nil)

	var receivedEv *LifecycleEvent
	var wg sync.WaitGroup
	wg.Add(1)

	fnWaker := NewFuncCallbackWaker("closure-waker", func(_ context.Context, ev *LifecycleEvent) error {
		receivedEv = ev
		wg.Done()
		return nil
	})
	assert.Equal(t, "closure-waker", fnWaker.Name())

	listener.RegisterWaker(fnWaker)

	ev := &LifecycleEvent{
		EventType: EventTypeSchedulerCallback,
		Kind:      "job",
		ID:        "job-lifecycle-999",
	}

	err = listener.processEvent(ev)
	require.NoError(t, err)

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("test_waiter", "wait for callback waker").StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		require.NotNil(t, receivedEv)
		assert.Equal(t, "job-lifecycle-999", receivedEv.ID)
		assert.Equal(t, EventTypeSchedulerCallback, receivedEv.EventType)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async callback waker dispatch")
	}
}

func TestListenerEndToEndWALReplaySchedulerCallback(t *testing.T) {
	tempDir := t.TempDir()
	wal, err := NewLifecycleEventWAL(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	ch := make(chan TransitionRequest, 10)
	listener := NewListener(tempDir, wal, nil, ch, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	var replayedEv *LifecycleEvent

	waker := NewFuncCallbackWaker("e2e-waker", func(_ context.Context, ev *LifecycleEvent) error {
		replayedEv = ev
		wg.Done()
		return nil
	})
	listener.RegisterWaker(waker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_listener_run", "run listener in background").StartSimple(func() {
		runErrCh <- listener.Run(ctx)
	})

	// Append scheduler_callback event to WAL
	err = wal.Append(&LifecycleEvent{
		EventType: EventTypeSchedulerCallback,
		Kind:      "job",
		ID:        "job-e2e-888",
	})
	require.NoError(t, err)
	err = wal.Sync()
	require.NoError(t, err)

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("test_waiter_e2e", "wait for e2e callback waker").StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		require.NotNil(t, replayedEv)
		assert.Equal(t, "job-e2e-888", replayedEv.ID)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for WAL replay to wake waker")
	}

	cancel()
	select {
	case err := <-runErrCh:
		assert.True(t, errors.Is(err, context.Canceled))
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not exit upon context cancellation")
	}

	// Verify checkpoint file exists
	checkpointPath := filepath.Join(tempDir, ".zqk", "wal", "lifecycle_events.wal.checkpoint")
	assert.FileExists(t, checkpointPath)
}
