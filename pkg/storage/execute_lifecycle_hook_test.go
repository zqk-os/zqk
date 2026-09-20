package storage

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestExecuteLifecycleHook_UsesInstalledHandlerOnly(t *testing.T) {
	t.Cleanup(func() { SetLifecycleHookHandler(nil) })

	var calls atomic.Int32
	SetLifecycleHookHandler(func(_ context.Context, kind, from, to string, _ map[string]any) error {
		calls.Add(1)
		if kind != "policy" || from != "draft" || to != "active" {
			t.Errorf("unexpected args kind=%s from=%s to=%s", kind, from, to)
		}
		return nil
	})
	if err := executeLifecycleHook(context.Background(), "policy", "draft", "active", map[string]any{}); err != nil {
		t.Fatalf("handler path: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls=%d want 1", calls.Load())
	}

	SetLifecycleHookHandler(nil)
	if err := executeLifecycleHook(context.Background(), "policy", "draft", "active", map[string]any{}); err != nil {
		t.Fatalf("nil handler: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("nil handler must not invoke previous callback, calls=%d", calls.Load())
	}

	SetLifecycleHookHandler(func(_ context.Context, kind, from, to string, _ map[string]any) error {
		calls.Add(1)
		return nil
	})
	if err := executeLifecycleHook(context.Background(), "policy", "active", "active", map[string]any{}); err != nil {
		t.Fatalf("same-state: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("same-state update must invoke handler, calls=%d want 2", calls.Load())
	}
	if err := executeLifecycleHook(context.Background(), "policy", "active", "erased", map[string]any{}); err != nil {
		t.Fatalf("erase transition: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("erase transition must invoke handler, calls=%d want 3", calls.Load())
	}
}
