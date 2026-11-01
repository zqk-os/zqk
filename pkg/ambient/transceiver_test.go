package ambient

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type mockInboxPusher struct {
	err    error
	events []ContextEvent
}

func (m *mockInboxPusher) Push(ctx context.Context, event ContextEvent) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}

// Static Floor Invariant: verify transceiver initialization and structural invariants.
func TestTransceiver_StaticFloorInvariant(t *testing.T) {
	mock := &mockInboxPusher{}
	tr := NewTransceiver(mock)

	if tr == nil {
		t.Fatal("expected non-nil Transceiver")
	}
	if tr.inbox == nil {
		t.Error("expected non-nil inbox")
	}
	if tr.builder == nil {
		t.Error("expected non-nil CSnapBuilder")
	}

	// Verify transceiver with nil builder self-heals upon EmitEnvelope
	trWithoutBuilder := &Transceiver{inbox: mock}
	err := trWithoutBuilder.EmitEnvelope(context.Background(), "test.go", []byte("package test"))
	if err != nil {
		t.Fatalf("expected nil error on self-healed builder, got: %v", err)
	}
	if trWithoutBuilder.builder == nil {
		t.Error("expected builder to be lazily instantiated")
	}
}

// Operational Dynamic Proof: verify Transceiver.EmitEnvelope pushing .csnap payloads to mock/in-memory InboxPusher.
func TestTransceiver_EmitEnvelope_OperationalDynamicProof(t *testing.T) {
	t.Run("push to mock inbox pusher", func(t *testing.T) {
		mock := &mockInboxPusher{}
		tr := NewTransceiver(mock)

		filePath := "pkg/ambient/engine.go"
		fileContent := []byte("package ambient\n\nfunc Run() {}\n")

		err := tr.EmitEnvelope(context.Background(), filePath, fileContent)
		if err != nil {
			t.Fatalf("EmitEnvelope failed: %v", err)
		}

		if len(mock.events) != 1 {
			t.Fatalf("expected 1 event pushed, got %d", len(mock.events))
		}

		evt := mock.events[0]
		if evt.ID != "csnap-temp-id" {
			t.Errorf("expected ID 'csnap-temp-id', got %q", evt.ID)
		}
		if evt.Type != "csnap" {
			t.Errorf("expected Type 'csnap', got %q", evt.Type)
		}

		var envelope CSnapEnvelope
		if err := json.Unmarshal(evt.Payload, &envelope); err != nil {
			t.Fatalf("failed to unmarshal CSnapEnvelope: %v", err)
		}

		if envelope.FilePath != filePath {
			t.Errorf("expected FilePath %q, got %q", filePath, envelope.FilePath)
		}
		if envelope.ASTPartial == "" {
			t.Error("expected non-empty ASTPartial")
		}
		if len(envelope.DependencyGraph) == 0 {
			t.Error("expected non-empty DependencyGraph")
		}
		if envelope.Timestamp <= 0 {
			t.Errorf("expected positive Timestamp, got %d", envelope.Timestamp)
		}
	})

	t.Run("push to AutonomyInbox ring buffer", func(t *testing.T) {
		inbox := NewAutonomyInbox(32)
		tr := NewTransceiver(inbox)

		err := tr.EmitEnvelope(context.Background(), "main.go", []byte("package main"))
		if err != nil {
			t.Fatalf("EmitEnvelope failed on AutonomyInbox: %v", err)
		}

		if inbox.Len() != 1 {
			t.Fatalf("expected 1 item in inbox, got %d", inbox.Len())
		}

		popped, ok := inbox.Pop()
		if !ok {
			t.Fatal("expected successful Pop from AutonomyInbox")
		}
		if popped.Type != "csnap" {
			t.Errorf("expected popped event Type 'csnap', got %q", popped.Type)
		}
	})
}

// Negative Adversarial Boundary: test nil inbox, error propagation from pusher, canceled context.
func TestTransceiver_NegativeAdversarialBoundary(t *testing.T) {
	t.Run("nil inbox error handling", func(t *testing.T) {
		tr := &Transceiver{inbox: nil, builder: NewCSnapBuilder()}
		err := tr.EmitEnvelope(context.Background(), "file.go", []byte("code"))
		if err == nil {
			t.Fatal("expected error when inbox is nil, got nil")
		}
		if err.Error() != "inbox is nil" {
			t.Errorf("expected 'inbox is nil' error, got %v", err)
		}
	})

	t.Run("pusher error propagation", func(t *testing.T) {
		expectedErr := errors.New("simulated push failure")
		mock := &mockInboxPusher{err: expectedErr}
		tr := NewTransceiver(mock)

		err := tr.EmitEnvelope(context.Background(), "file.go", []byte("code"))
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("canceled context with AutonomyInbox", func(t *testing.T) {
		inbox := NewAutonomyInbox(32)
		tr := NewTransceiver(inbox)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := tr.EmitEnvelope(ctx, "file.go", []byte("code"))
		if err == nil {
			t.Fatal("expected error on canceled context, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}
