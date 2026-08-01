package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockIdleDetector struct {
	mu     sync.Mutex
	isIdle bool
}

func (m *mockIdleDetector) IsIdle(ctx context.Context, agentID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isIdle, nil
}

func (m *mockIdleDetector) MarkActive(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isIdle = false
	return nil
}

type mockChatResponder struct {
	mu          sync.Mutex
	engagements int
	lastMessage string
}

func (m *mockChatResponder) Engage(ctx context.Context, agentID string, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.engagements++
	m.lastMessage = message
	return nil
}

func TestIdlePreventionMiddleware(t *testing.T) {
	t.Parallel()

	detector := &mockIdleDetector{isIdle: true}
	responder := &mockChatResponder{}

	opts := IdlePreventionOptions{
		Detector:      detector,
		Responder:     responder,
		IdleThreshold: 10 * time.Millisecond,
		WakeupMessage: "Wake up!",
	}

	nextInvoked := false
	nextStage := func(pctx *Context, payload any) (any, error) {
		nextInvoked = true
		// sleep a bit to allow ticker to fire
		time.Sleep(30 * time.Millisecond)
		return payload, nil
	}

	middleware := IdlePreventionMiddleware("agent-123", opts, nextStage)
	pctx := &Context{Ctx: context.Background()}

	_, err := middleware(pctx, "payload")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !nextInvoked {
		t.Fatalf("expected wrapped stage to be invoked")
	}

	responder.mu.Lock()
	engagements := responder.engagements
	lastMessage := responder.lastMessage
	responder.mu.Unlock()

	if engagements == 0 {
		t.Fatalf("expected chat responder to be engaged at least once")
	}

	if lastMessage != "Wake up!" {
		t.Fatalf("expected lastMessage 'Wake up!', got %v", lastMessage)
	}
}

func TestIdlePreventionMiddleware_MissingDeps(t *testing.T) {
	t.Parallel()

	opts := IdlePreventionOptions{} // Missing detector/responder
	middleware := IdlePreventionMiddleware("agent-123", opts, func(pctx *Context, payload any) (any, error) {
		return payload, nil
	})

	pctx := &Context{Ctx: context.Background()}
	_, err := middleware(pctx, "payload")
	if err == nil {
		t.Fatalf("expected error due to missing dependencies")
	}
}
