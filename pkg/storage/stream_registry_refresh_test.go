package storage

import (
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestStreamRegistrySnapshot_ConcurrentRefreshDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	snap := &streamRegistrySnapshot{
		locations: make(map[string]string),
		deleted:   make(map[string]bool),
	}
	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		goroutinelabels.NewGoroutine("storage_test", "concurrent stream registry refresh").StartSimple(func() {
			defer wg.Done()
			snap.refresh(tmp, "base_metric")
		})
	}
	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("storage_test", "wait concurrent stream registry refresh").StartSimple(func() {
		wg.Wait()
		close(waitDone)
	})
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent refresh")
	}
}

func TestStreamRegistrySnapshot_RefreshCoalescesWithinInterval(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	snap := &streamRegistrySnapshot{
		locations: make(map[string]string),
		deleted:   make(map[string]bool),
	}
	snap.refresh(tmp, "base_metric")
	first := snap.lastRefresh
	if first.IsZero() {
		t.Fatal("expected lastRefresh after first refresh")
	}
	time.Sleep(time.Millisecond)
	snap.refresh(tmp, "base_metric")
	if !snap.lastRefresh.Equal(first) {
		t.Fatalf("coalesced refresh moved lastRefresh from %v to %v", first, snap.lastRefresh)
	}
}
