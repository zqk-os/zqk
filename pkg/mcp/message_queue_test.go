package mcp

import (
	"bufio"
	"bytes"
	"sync"
	"testing"
)

type blockingWriter struct {
	mu          sync.Mutex
	buf         bytes.Buffer
	startedOnce sync.Once
	started     chan struct{}
	unblock     chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.startedOnce.Do(func() {
		close(w.started)
	})
	<-w.unblock
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func TestMessageQueue_AtomicCounters(t *testing.T) {
	bw := &blockingWriter{
		started: make(chan struct{}),
		unblock: make(chan struct{}),
	}
	// Buffer size 1 ensures the first write flushes straight into blockingWriter,
	// deterministically pausing the background consumer so queue capacity fills without racing.
	writer := bufio.NewWriterSize(bw, 1)
	format := &MessageFormat{IsRawJSON: true}
	config := QueueConfig{
		MaxQueueSize:  2,
		DropWhenFull:  true,
		FlushInterval: 0,
	}

	queue := NewMessageQueue(writer, format, config)
	defer queue.Stop()
	defer close(bw.unblock)

	msgData := []byte(`{"jsonrpc":"2.0","method":"test"}` + "\n")

	// Enqueue 1st message and wait until the consumer goroutine is parked in blockingWriter.
	if ok := queue.Enqueue(msgData, format, "normal", nil); !ok {
		t.Fatalf("expected 1st message enqueue to succeed")
	}
	<-bw.started

	// Fill both buffer slots while the consumer is blocked on message #1.
	if ok := queue.Enqueue(msgData, format, "normal", nil); !ok {
		t.Fatalf("expected 2nd message enqueue to succeed")
	}
	if ok := queue.Enqueue(msgData, format, "normal", nil); !ok {
		t.Fatalf("expected 3rd message enqueue to succeed")
	}

	// Next enqueue must deterministically drop because the queue is full.
	enqueued := queue.Enqueue(msgData, format, "normal", nil)
	if enqueued {
		t.Errorf("expected 4th message enqueue to return false (dropped)")
	}

	stats := queue.Stats()
	if stats.Dropped != 1 {
		t.Errorf("expected stats.Dropped=1, got %d", stats.Dropped)
	}
}
