package idehooks

import (
	"context"
	"errors"
	"testing"
)

func TestIDEHooksManager_Dispatch(t *testing.T) {
	m := NewManager()

	var preToolUseCalled bool
	var postToolUseCalled bool

	m.Register(PreToolUse, InterceptorFunc(func(ctx context.Context, event Event) error {
		preToolUseCalled = true
		if event.Type != PreToolUse {
			t.Errorf("expected PreToolUse event, got %v", event.Type)
		}
		return nil
	}))

	m.Register(PostToolUse, InterceptorFunc(func(ctx context.Context, event Event) error {
		postToolUseCalled = true
		return nil
	}))

	err := m.Dispatch(context.Background(), Event{
		Type:    PreToolUse,
		Payload: "test payload",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !preToolUseCalled {
		t.Error("expected PreToolUse interceptor to be called")
	}

	if postToolUseCalled {
		t.Error("expected PostToolUse interceptor to NOT be called")
	}
}

func TestIDEHooksManager_Dispatch_Error(t *testing.T) {
	m := NewManager()

	expectedErr := errors.New("interceptor error")

	m.Register(PreInvocation, InterceptorFunc(func(ctx context.Context, event Event) error {
		return expectedErr
	}))

	var secondInterceptorCalled bool
	m.Register(PreInvocation, InterceptorFunc(func(ctx context.Context, event Event) error {
		secondInterceptorCalled = true
		return nil
	}))

	err := m.Dispatch(context.Background(), Event{
		Type: PreInvocation,
	})

	if err != expectedErr {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	if secondInterceptorCalled {
		t.Error("expected second interceptor to NOT be called due to abort")
	}
}

func TestIDEHooksManager_AllHookTypes(t *testing.T) {
	// Just verify all consts are defined
	hookTypes := []HookType{
		PreToolUse,
		PostToolUse,
		PreInvocation,
		PostInvocation,
		Stop,
	}

	if len(hookTypes) != 5 {
		t.Errorf("Expected 5 hook types, found %d", len(hookTypes))
	}
}
