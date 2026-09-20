package system

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// waitUntilProjectCacheBgScheduled blocks until triggerBackgroundObjectIDCacheBuild has registered
// background state for root, or until deadline. When [DefaultBudget] is non-nil, Inc runs inside
// the worker goroutine; [WaitProjectCacheBackgroundWork] returns immediately if no state exists yet,
// which races the test that expects idle callbacks (flake under load).
func waitUntilProjectCacheBgScheduled(t *testing.T, root string) {
	t.Helper()
	root = ProjectRootOrResolveDot(root)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if projectCacheBgHas(root) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for background cache state for %q", root)
}

func TestWaitProjectCacheBackgroundWork_NoWorkIsImmediate(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitProjectCacheBackgroundWork(ctx, root); err != nil {
		t.Fatalf("WaitProjectCacheBackgroundWork: %v", err)
	}
}

func TestWaitProjectCacheBackgroundWork_JoinsBackgroundObjectIDBuild(t *testing.T) {
	root := ProjectRootOrResolveDot(t.TempDir())
	TriggerBackgroundObjectIDCacheBuild(root)
	waitUntilProjectCacheBgScheduled(t, root)
	if err := WaitProjectCacheBackgroundWork(context.Background(), root); err != nil {
		t.Fatalf("WaitProjectCacheBackgroundWork: %v", err)
	}
}

func TestRegisterCacheSidecarsIdleCallback_FiresWhenBackgroundCompletes(t *testing.T) {
	root := ProjectRootOrResolveDot(t.TempDir())
	var sawRoot atomic.Value // string
	remove := RegisterCacheSidecarsIdleCallback(func(projectRoot string) {
		sawRoot.Store(projectRoot)
	})
	defer remove()

	TriggerBackgroundObjectIDCacheBuild(root)
	waitUntilProjectCacheBgScheduled(t, root)
	if err := WaitProjectCacheBackgroundWork(context.Background(), root); err != nil {
		t.Fatalf("WaitProjectCacheBackgroundWork: %v", err)
	}
	// Idle callbacks run after unlock (see notifyCacheSidecarsIdle after projectCacheBgDec*); Wait can
	// return before callbacks run — poll for the expected root (generous bound for loaded CI/scheduler).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if got, ok := sawRoot.Load().(string); ok && got == root {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got, _ := sawRoot.Load().(string); got != root {
		t.Fatalf("idle callback projectRoot: got %q want %q", got, root)
	}
}
