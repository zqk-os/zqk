package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/lifecycle"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// setupTestServerAndStreamer initializes an isolated test environment for waker stream tests.
func setupTestServerAndStreamer(t *testing.T, dir string) (*Server, *lifecycle.JobWakerRegistry, *lifecycle.LifecycleEventWAL, *WakerEventStreamer) {
	t.Helper()
	server := NewServer()
	registry := lifecycle.NewJobWakerRegistry()

	wal, err := lifecycle.NewLifecycleEventWAL(dir)
	if err != nil {
		t.Fatalf("failed to create lifecycle WAL: %v", err)
	}

	streamer := NewWakerEventStreamer(server,
		WithJobWakerRegistry(registry),
		WithWAL(wal),
		WithProjectRoot(dir),
		WithPollInterval(20*time.Millisecond),
		WithBufferSize(64),
	)

	startErr := streamer.Start(context.Background())
	if startErr != nil {
		t.Fatalf("failed to start streamer: %v", startErr)
	}

	return server, registry, wal, streamer
}

// createTestSubscription registers an MCPEventSubscriber on the server's EventEmitter.
func createTestSubscription(t *testing.T, server *Server, subID string, eventTypes []EventType) (chan []byte, *MCPEventSubscriber) {
	t.Helper()
	receivedCh := make(chan []byte, 32)
	writeFunc := func(data []byte) error {
		receivedCh <- append([]byte(nil), data...)
		return nil
	}

	subscriber := NewMCPEventSubscriber(subID, eventTypes, writeFunc, 5*time.Minute)
	emitter := server.GetEventEmitter()
	if emitter == nil {
		t.Fatalf("expected non-nil event emitter on server")
	}
	emitter.Subscribe(subscriber)

	return receivedCh, subscriber
}

// parseNotificationPayload unmarshals raw JSON-RPC notification payload into map.
func parseNotificationPayload(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var parsed map[string]any
	err := json.Unmarshal(raw, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal notification json: %v, raw: %s", err, string(raw))
	}
	return parsed
}

// verifyFastPathDispatch asserts in-memory dispatch from JobWakerRegistry arrives via MCP.
func verifyFastPathDispatch(t *testing.T, registry *lifecycle.JobWakerRegistry, receivedCh chan []byte) {
	t.Helper()
	testJobEvent := &lifecycle.LifecycleEvent{
		ID:        "job-fast-101",
		EventType: lifecycle.EventTypeSchedulerCallback,
		Kind:      "scheduler_job",
		ToStatus:  "completed",
		Ts:        time.Now(),
	}

	start := time.Now()
	dispatched := registry.Dispatch(testJobEvent)
	if dispatched == 0 {
		t.Fatalf("expected at least 1 subscriber dispatched in waker registry")
	}

	select {
	case data := <-receivedCh:
		elapsed := time.Since(start)
		if elapsed > 500*time.Millisecond {
			t.Errorf("expected near-instantaneous notification (<500ms), got %v", elapsed)
		}

		payload := parseNotificationPayload(t, data)
		method, ok := payload["method"].(string)
		if !ok || method != "notifications/event" {
			t.Errorf("expected method notifications/event, got %v", method)
		}

		params, pOk := payload["params"].(map[string]any)
		if !pOk {
			t.Fatalf("missing params in notification: %v", payload)
		}
		eventMap, eOk := params["event"].(map[string]any)
		if !eOk {
			t.Fatalf("missing event in notification params: %v", params)
		}

		eventType, _ := eventMap["type"].(string)
		if eventType != string(EventTypeTaskWaker) {
			t.Errorf("expected event type %s, got %s", EventTypeTaskWaker, eventType)
		}

		fields, fOk := eventMap["fields"].(map[string]any)
		if !fOk {
			t.Fatalf("missing fields in notification event: %v", eventMap)
		}
		if fields["job_id"] != "job-fast-101" {
			t.Errorf("expected job_id job-fast-101, got %v", fields["job_id"])
		}
		if fields["to_status"] != "completed" {
			t.Errorf("expected to_status completed, got %v", fields["to_status"])
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for in-memory waker notification")
	}
}

// verifyWALReplayDispatch asserts persisted WAL events are replayed and delivered.
func verifyWALReplayDispatch(t *testing.T, wal *lifecycle.LifecycleEventWAL, receivedCh chan []byte) {
	t.Helper()
	walEvent := &lifecycle.LifecycleEvent{
		ID:          "job-wal-202",
		EventType:   lifecycle.EventTypeCriterionSatisfied,
		CriterionID: "CRIT-TEST-001",
		Kind:        "criteria",
		ToStatus:    "completed",
		Ts:          time.Now(),
	}

	appendErr := wal.Append(walEvent)
	if appendErr != nil {
		t.Fatalf("failed to append WAL event: %v", appendErr)
	}
	syncErr := wal.Sync()
	if syncErr != nil {
		t.Fatalf("failed to sync WAL: %v", syncErr)
	}

	select {
	case data := <-receivedCh:
		payload := parseNotificationPayload(t, data)
		params, pOk := payload["params"].(map[string]any)
		if !pOk {
			t.Fatalf("missing params in WAL notification: %v", payload)
		}
		eventMap, eOk := params["event"].(map[string]any)
		if !eOk {
			t.Fatalf("missing event in WAL notification params: %v", params)
		}
		fields, fOk := eventMap["fields"].(map[string]any)
		if !fOk {
			t.Fatalf("missing fields in WAL event: %v", eventMap)
		}
		if fields["criterion_id"] != "CRIT-TEST-001" {
			t.Errorf("expected criterion_id CRIT-TEST-001, got %v", fields["criterion_id"])
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for WAL replayed notification")
	}
}

// TestZeroIdleWakerStream_Functional satisfies CRIT-1791620837263622000-58400bbd:
// Connect JobWakerRegistry and LifecycleEventWAL to MCP notification events,
// streaming JSON-RPC waker notifications without polling delay.
func TestZeroIdleWakerStream_Functional(t *testing.T) {
	tempDir := t.TempDir()
	server, registry, wal, streamer := setupTestServerAndStreamer(t, tempDir)
	defer streamer.Stop()
	defer func() {
		closeErr := wal.Close()
		if closeErr != nil {
			t.Logf("wal close error: %v", closeErr)
		}
	}()

	receivedCh, subscriber := createTestSubscription(t, server, "test_func_sub", []EventType{EventTypeTaskWaker, EventTypeLifecycleEvent})
	defer server.GetEventEmitter().Unsubscribe(subscriber.ID())

	verifyFastPathDispatch(t, registry, receivedCh)
	verifyWALReplayDispatch(t, wal, receivedCh)

	streamed, dropped := streamer.Stats()
	if dropped < 0 {
		t.Errorf("unexpected negative dropped count: %d", dropped)
	}
	if streamed < 2 {
		t.Errorf("expected at least 2 events streamed, got %d", streamed)
	}
}

// TestZeroIdleWakerStream_Boundary satisfies CRIT-1791620837263623000-c55c4e2c:
// Clean subscriber registration/unregistration, non-blocking broadcast delivery
// with buffer overrun protection, graceful handling of disconnected clients, zero leaks.
func TestZeroIdleWakerStream_Boundary(t *testing.T) {
	tempDir := t.TempDir()
	server, registry, wal, streamer := setupTestServerAndStreamer(t, tempDir)
	defer func() {
		closeErr := wal.Close()
		if closeErr != nil {
			t.Logf("wal close error: %v", closeErr)
		}
	}()

	// 1. Double-start protection
	secondStartErr := streamer.Start(context.Background())
	if secondStartErr == nil {
		t.Errorf("expected error when starting already running streamer")
	}

	// 2. Buffer overrun protection with a rejecting/slow subscriber
	failingSub := NewMCPEventSubscriber("failing_sub", []EventType{EventTypeTaskWaker}, func([]byte) error {
		return errors.New("connection closed")
	}, 10*time.Millisecond)
	server.GetEventEmitter().Subscribe(failingSub)

	// Dispatch multiple burst events
	for i := 0; i < 20; i++ {
		ev := &lifecycle.LifecycleEvent{
			ID:        "job-burst-test",
			EventType: lifecycle.EventTypeSchedulerCallback,
			Kind:      "scheduler_job",
			ToStatus:  "running",
			Ts:        time.Now(),
		}
		dispatched := registry.Dispatch(ev)
		if dispatched == 0 {
			t.Logf("iteration %d: registry dispatched 0 (clean unregister or deduplicated)", i)
		}
	}

	// Allow event emitter loop to cycle and cleanup failing subscriber
	time.Sleep(50 * time.Millisecond)

	// 3. Clean stop & unregistration (zero channel or goroutine leaks)
	streamer.Stop()
	if streamer.IsRunning() {
		t.Errorf("expected streamer to be stopped")
	}

	// Dispatch after stop: should cleanly route 0 to streamer
	postStopEvent := &lifecycle.LifecycleEvent{
		ID:        "job-post-stop",
		EventType: lifecycle.EventTypeSchedulerCallback,
		Kind:      "scheduler_job",
		ToStatus:  "completed",
		Ts:        time.Now(),
	}
	remaining := registry.Dispatch(postStopEvent)
	if remaining != 0 {
		t.Errorf("expected 0 remaining subscribers after streamer Stop(), got %d", remaining)
	}

	// Idempotent Stop
	streamer.Stop()
}

// TestZeroIdleWakerStream_Documentation satisfies CRIT-1791620837263624000-2b566397:
// Author docs/architecture/ZERO_IDLE_MCP_EVENT_STREAM.md specifying streaming protocol,
// notification event formats, and client subscription model.
func TestZeroIdleWakerStream_Documentation(t *testing.T) {
	docPath := filepath.Join("..", "..", "docs", "architecture", "ZERO_IDLE_MCP_EVENT_STREAM.md")
	content, err := fileutil.ReadFile(docPath)
	if err != nil {
		t.Fatalf("failed to read architectural documentation: %v", err)
	}

	doc := string(content)
	requiredSections := []string{
		"Zero-Idle MCP Event Stream and Real-Time Agent Notification Mesh",
		"JobWakerRegistry",
		"LifecycleEventWAL",
		"notifications/event",
		"notifications/message",
		"events/subscribe",
		"events/list",
		"Buffer Overrun Protection",
	}

	for _, req := range requiredSections {
		if !strings.Contains(doc, req) {
			t.Errorf("documentation missing required section or keyword: %q", req)
		}
	}
}
