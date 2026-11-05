package events

import (
	"context"
	"errors"
	"testing"
)

func TestTopicBus_PublishSubscribe(t *testing.T) {
	bus := NewTopicBus(10)

	var received []string
	unsub := bus.Subscribe("test.topic", func(ctx context.Context, topic string, payload any) error {
		received = append(received, payload.(string))
		return nil
	})

	err := bus.Publish(context.Background(), "test.topic", "hello")
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if len(received) != 1 || received[0] != "hello" {
		t.Fatalf("expected ['hello'], got: %v", received)
	}

	// Verify history
	hist := bus.History("test.topic")
	if len(hist) != 1 || hist[0].Payload.(string) != "hello" {
		t.Fatalf("expected 1 history record, got: %v", hist)
	}

	unsub()

	// Clear history
	bus.ClearHistory()
	if len(bus.History("")) != 0 {
		t.Fatalf("expected empty history after ClearHistory")
	}
}

func TestTopicBus_SubscribeAll(t *testing.T) {
	bus := NewTopicBus(10)

	var count int
	bus.SubscribeAll(func(ctx context.Context, topic string, payload any) error {
		count++
		return nil
	})

	_ = bus.Publish(context.Background(), "topic.one", 1)
	_ = bus.Publish(context.Background(), "topic.two", 2)

	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}
}

func TestTopicBus_HandlerError(t *testing.T) {
	bus := NewTopicBus(10)
	expectedErr := errors.New("handler failed")

	bus.Subscribe("error.topic", func(ctx context.Context, topic string, payload any) error {
		return expectedErr
	})

	err := bus.Publish(context.Background(), "error.topic", "fail")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestGetDefaultBus(t *testing.T) {
	b1 := GetDefaultBus()
	b2 := GetDefaultBus()
	if b1 == nil || b1 != b2 {
		t.Fatalf("GetDefaultBus should return singleton instance")
	}
}
