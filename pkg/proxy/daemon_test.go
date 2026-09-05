package proxy

import (
	"context"
	"errors"
	"testing"
)

type MockForwarder struct {
	ForwardFunc func(ctx context.Context, req []byte) ([]byte, error)
}

func (m *MockForwarder) Forward(ctx context.Context, req []byte) ([]byte, error) {
	if m.ForwardFunc != nil {
		return m.ForwardFunc(ctx, req)
	}
	return nil, nil
}

type MockHeartbeat struct {
	PulseFunc func(ctx context.Context) error
}

func (m *MockHeartbeat) Pulse(ctx context.Context) error {
	if m.PulseFunc != nil {
		return m.PulseFunc(ctx)
	}
	return nil
}

func TestDaemon_Start(t *testing.T) {
	t.Run("successfully starts and pulses", func(t *testing.T) {
		ctx := context.Background()
		pulled := false

		f := &MockForwarder{}
		h := &MockHeartbeat{
			PulseFunc: func(ctx context.Context) error {
				pulled = true
				return nil
			},
		}

		d := NewDaemon(f, h)
		err := d.Start(ctx)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !pulled {
			t.Fatalf("expected heartbeat pulse to be called")
		}
	})

	t.Run("handles heartbeat failure", func(t *testing.T) {
		ctx := context.Background()
		f := &MockForwarder{}
		expectedErr := errors.New("connection reset")

		h := &MockHeartbeat{
			PulseFunc: func(ctx context.Context) error {
				return expectedErr
			},
		}

		d := NewDaemon(f, h)
		err := d.Start(ctx)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, expectedErr) && err.Error() != "heartbeat failed: connection reset" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("handles context cancellation via InterruptChecker", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // immediately cancel

		f := &MockForwarder{}
		h := &MockHeartbeat{}

		d := NewDaemon(f, h)
		err := d.Start(ctx)
		if err == nil {
			t.Fatal("expected error due to canceled context, got nil")
		}
		if !errors.Is(err, ErrContextCanceled) {
			t.Fatalf("expected ErrContextCanceled, got: %v", err)
		}
	})
}
