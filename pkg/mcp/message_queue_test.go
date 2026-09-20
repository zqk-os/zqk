package mcp

import (
	"bufio"
	"bytes"
	"testing"
)

func TestMessageQueue_AtomicCounters(t *testing.T) {
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: true}
	config := QueueConfig{
		MaxQueueSize:  2,
		DropWhenFull:  true,
		FlushInterval: 0,
	}

	queue := NewMessageQueue(writer, format, config)
	defer queue.Stop()

	// Enqueue messages until full + dropped
	msgData := []byte(`{"jsonrpc":"2.0","method":"test"}` + "\n")
	_ = queue.Enqueue(msgData, format, "normal", nil)

	// Fill queue and trigger drop
	_ = queue.Enqueue(msgData, format, "normal", nil)
	dropped := queue.Enqueue(msgData, format, "normal", nil)

	if dropped {
		t.Errorf("expected 3rd message enqueue to return false (dropped)")
	}

	stats := queue.Stats()
	if stats.Dropped != 1 {
		t.Errorf("expected stats.Dropped=1, got %d", stats.Dropped)
	}
}
