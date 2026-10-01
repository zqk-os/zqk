package validation

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestOutputQueue_EnqueueHelpersAndError(t *testing.T) {
	q := NewOutputQueue(2)

	// Test EnqueueProgress
	progress := ValidationProgress{
		CurrentObject: "item_1",
		Status:        "validating",
		Errors:        nil,
	}
	if err := q.EnqueueProgress(progress, false); err != nil {
		t.Fatalf("EnqueueProgress failed: %v", err)
	}

	// Test EnqueueMetrics
	if err := q.EnqueueMetrics(map[string]any{"rate": 100}, false); err != nil {
		t.Fatalf("EnqueueMetrics failed: %v", err)
	}

	// Queue is full now (maxSize = 2)
	err := q.EnqueueStdout("hello", false)
	if err == nil {
		t.Fatal("expected queue full error, got nil")
	}
	if err.Error() != ErrQueueFull.Error() {
		t.Errorf("expected ErrQueueFull error string, got %v", err)
	}

	// Dequeue one item
	packet := q.Dequeue()
	if packet.ChannelID != "progress" {
		t.Errorf("expected channel progress, got %s", packet.ChannelID)
	}

	// Now EnqueueStderr succeeds
	if err := q.EnqueueStderr("err data", true); err != nil {
		t.Fatalf("EnqueueStderr failed: %v", err)
	}
}

func TestOutputWriter_Lifecycle(t *testing.T) {
	q := NewOutputQueue(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := NewOutputWriter(ctx, q, nil)

	var buf bytes.Buffer
	handler := NewWriterOutputHandler(&buf, false)
	writer.RegisterHandler("stdout", handler)

	writer.Start()

	_ = q.EnqueueStdout("line 1\n", false)
	_ = q.EnqueueStdout([]byte("line 2\n"), true)
	_ = q.EnqueueStdout(12345, false)

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	if err := writer.Stop(); err != nil {
		t.Fatalf("writer.Stop failed: %v", err)
	}

	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("line 1")) || !bytes.Contains([]byte(out), []byte("12345")) {
		t.Errorf("unexpected output buffer: %s", out)
	}
}

type errorOutputHandler struct{}

func (e *errorOutputHandler) Write(data any) error {
	return errors.New("write error")
}

func (e *errorOutputHandler) Flush() error {
	return errors.New("flush error")
}

func (e *errorOutputHandler) Close() error {
	return nil
}

func TestOutputWriter_ErrorHandling(t *testing.T) {
	q := NewOutputQueue(5)
	writer := NewOutputWriter(context.Background(), q, nil)
	writer.RegisterHandler("err_ch", &errorOutputHandler{})

	writer.processBatch([]OutputPacket{
		{ChannelID: "err_ch", Data: "test", FlushHint: true},
		{ChannelID: "unknown_ch", Data: "test"},
	})
	writer.flushAll()
}
