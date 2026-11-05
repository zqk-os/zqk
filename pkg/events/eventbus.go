package events

import (
	"context"
	"sync"
	"time"
)

// EventHandler is a callback invoked when an event is published to a subscribed topic.
type EventHandler func(ctx context.Context, topic string, payload any) error

// EventRecord captures an emitted event for auditing and testing.
type EventRecord struct {
	Topic     string    `json:"topic"`
	Payload   any       `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

// EventBus defines the pub-sub contract for the event stream.
type EventBus interface {
	Publish(ctx context.Context, topic string, payload any) error
	Subscribe(topic string, handler EventHandler) func()
	SubscribeAll(handler EventHandler) func()
	History(topic string) []EventRecord
	ClearHistory()
}

// TopicBus is an in-memory, thread-safe implementation of EventBus.
type TopicBus struct {
	mu          sync.RWMutex
	handlers    map[string][]EventHandler
	allHandlers []EventHandler
	history     []EventRecord
	maxHistory  int
}

var (
	defaultBus     *TopicBus
	defaultBusOnce sync.Once
)

// GetDefaultBus returns the singleton system event bus.
func GetDefaultBus() *TopicBus {
	defaultBusOnce.Do(func() {
		defaultBus = NewTopicBus(1000)
	})
	return defaultBus
}

// NewTopicBus creates a new TopicBus with history tracking.
func NewTopicBus(maxHistory int) *TopicBus {
	if maxHistory <= 0 {
		maxHistory = 500
	}
	return &TopicBus{
		handlers:   make(map[string][]EventHandler),
		maxHistory: maxHistory,
	}
}

// Subscribe registers a handler for a specific topic.
// Returns an unsubscribe function.
func (b *TopicBus) Subscribe(topic string, handler EventHandler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[topic] = append(b.handlers[topic], handler)

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		list := b.handlers[topic]
		for i, h := range list {
			// Compare function pointers
			if &h == &handler {
				b.handlers[topic] = append(list[:i], list[i+1:]...)
				break
			}
		}
	}
}

// SubscribeAll registers a handler that receives all topics.
func (b *TopicBus) SubscribeAll(handler EventHandler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.allHandlers = append(b.allHandlers, handler)

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, h := range b.allHandlers {
			if &h == &handler {
				b.allHandlers = append(b.allHandlers[:i], b.allHandlers[i+1:]...)
				break
			}
		}
	}
}

// Publish emits a payload to all subscribers of the topic and records it in history.
func (b *TopicBus) Publish(ctx context.Context, topic string, payload any) error {
	b.mu.Lock()
	rec := EventRecord{
		Topic:     topic,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}
	b.history = append(b.history, rec)
	if len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}

	// Copy handlers to execute outside write lock
	var targetHandlers []EventHandler
	if topicHandlers, ok := b.handlers[topic]; ok {
		targetHandlers = append(targetHandlers, topicHandlers...)
	}
	targetHandlers = append(targetHandlers, b.allHandlers...)
	b.mu.Unlock()

	for _, handler := range targetHandlers {
		if err := handler(ctx, topic, payload); err != nil {
			return err
		}
	}
	return nil
}

// History returns a copy of past events, optionally filtered by topic.
func (b *TopicBus) History(topic string) []EventRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if topic == "" {
		out := make([]EventRecord, len(b.history))
		copy(out, b.history)
		return out
	}

	var filtered []EventRecord
	for _, rec := range b.history {
		if rec.Topic == topic {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

// ClearHistory resets the recorded events.
func (b *TopicBus) ClearHistory() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.history = nil
}
