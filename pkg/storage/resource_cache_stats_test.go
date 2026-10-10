package storage

import (
	"context"
	"errors"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResourceCache_LifetimeCounters(t *testing.T) {
	t.Parallel()

	var nilRc *ResourceCache[string]
	hNil, mNil, cNil := nilRc.GetResourceCacheStats()
	if hNil != 0 || mNil != 0 || cNil != 0 {
		t.Fatalf("expected nil stats (0, 0, 0), got (%d, %d, %d)", hNil, mNil, cNil)
	}

	rc := &ResourceCache[string]{}

	ctx := context.Background()

	// 1. GetOrCreate first time -> miss (1), creation (1)
	val1, err := rc.GetOrCreate(ctx, "k1", func(ctx context.Context, key string) (string, error) {
		return "val-1", nil
	})
	if err != nil || val1 != "val-1" {
		t.Fatalf("unexpected GetOrCreate result: val=%s err=%v", val1, err)
	}

	h1, m1, c1 := rc.GetResourceCacheStats()
	if h1 != 0 || m1 != 1 || c1 != 1 {
		t.Fatalf("expected stats (0, 1, 1), got (%d, %d, %d)", h1, m1, c1)
	}

	// 2. GetOrCreate second time -> hit (1)
	val2, err := rc.GetOrCreate(ctx, "k1", func(ctx context.Context, key string) (string, error) {
		return "val-2", nil
	})
	if err != nil || val2 != "val-1" {
		t.Fatalf("unexpected GetOrCreate second result: val=%s err=%v", val2, err)
	}

	h2, m2, c2 := rc.GetResourceCacheStats()
	if h2 != 1 || m2 != 1 || c2 != 1 {
		t.Fatalf("expected stats (1, 1, 1), got (%d, %d, %d)", h2, m2, c2)
	}

	// 3. Get existing -> hit (2)
	val3, found := rc.Get("k1")
	if !found || val3 != "val-1" {
		t.Fatalf("expected Get hit for k1")
	}

	// 4. Get missing -> miss (2)
	_, foundMissing := rc.Get("missing")
	if foundMissing {
		t.Fatalf("expected Get miss for missing key")
	}

	hFinal, mFinal, cFinal := rc.GetResourceCacheStats()
	if hFinal != 2 || mFinal != 2 || cFinal != 1 {
		t.Fatalf("expected final stats (2, 2, 1), got (%d, %d, %d)", hFinal, mFinal, cFinal)
	}
}

func TestStorageProviderCache_LifetimeCounters(t *testing.T) {
	t.Parallel()

	var nilSpc *StorageProviderCache
	hNil, mNil, cNil := nilSpc.GetStorageProviderCacheStats()
	if hNil != 0 || mNil != 0 || cNil != 0 {
		t.Fatalf("expected nil StorageProviderCache stats (0, 0, 0), got (%d, %d, %d)", hNil, mNil, cNil)
	}

	spc := NewStorageProviderCache()
	if spc == nil {
		t.Fatalf("expected NewStorageProviderCache non-nil")
	}

	hInit, mInit, cInit := spc.GetStorageProviderCacheStats()
	if hInit != 0 || mInit != 0 || cInit != 0 {
		t.Fatalf("expected initial StorageProviderCache stats (0, 0, 0), got (%d, %d, %d)", hInit, mInit, cInit)
	}
}

func TestResourceCache_TransientErrorEviction(t *testing.T) {
	t.Parallel()

	rc := &ResourceCache[string]{}
	ctx := context.Background()

	attempt := 0
	initFn := func(ctx context.Context, key string) (string, error) {
		attempt++
		if attempt == 1 {
			return "", context.DeadlineExceeded
		}
		return "recovered-resource", nil
	}

	// First attempt fails with transient error
	val, err := rc.GetOrCreate(ctx, "res-1", initFn)
	if err == nil {
		t.Fatalf("expected transient error on first attempt, got val=%s", val)
	}

	// Entry must NOT be permanently poisoned: second attempt must retry and succeed
	val2, err2 := rc.GetOrCreate(ctx, "res-1", initFn)
	if err2 != nil {
		t.Fatalf("expected success on second attempt after transient failure, got err: %v", err2)
	}
	if val2 != "recovered-resource" {
		t.Fatalf("expected recovered-resource, got %s", val2)
	}
}

func TestResourceCache_InFlightGetSafety(t *testing.T) {
	t.Parallel()

	rc := &ResourceCache[string]{}
	ctx := context.Background()

	started := make(chan struct{})
	release := make(chan struct{})

	goroutinelabels.NewGoroutine("resource_cache_test", "in-flight get safety worker").StartSimple(func() {
		if _, err := rc.GetOrCreate(ctx, "inflight-key", func(ctx context.Context, key string) (string, error) {
			close(started)
			<-release
			return "initialized-value", nil
		}); err != nil {
			t.Errorf("unexpected error in background GetOrCreate: %v", err)
		}
	})

	<-started

	// While initialization is in flight, Get must return false and NOT zero-value as a hit.
	val, ok := rc.Get("inflight-key")
	if ok {
		t.Fatalf("expected Get during in-flight initialization to return false, got ok=true val=%q", val)
	}

	// Unblock initialization
	close(release)

	// Now wait for completion via GetOrCreate
	res, err := rc.GetOrCreate(ctx, "inflight-key", func(ctx context.Context, key string) (string, error) {
		return "should-not-be-called", nil
	})
	if err != nil || res != "initialized-value" {
		t.Fatalf("expected initialized-value after release, got val=%q, err=%v", res, err)
	}

	// Get should now succeed
	valAfter, okAfter := rc.Get("inflight-key")
	if !okAfter || valAfter != "initialized-value" {
		t.Fatalf("expected Get after initialization to return initialized-value, got ok=%v val=%q", okAfter, valAfter)
	}
}

func TestResourceCache_ConcurrentRetryOnTransientFailure(t *testing.T) {
	t.Parallel()

	rc := &ResourceCache[string]{}
	ctx := context.Background()

	var attempts atomic.Int32
	firstAttemptStarted := make(chan struct{})
	firstAttemptBlock := make(chan struct{})

	initFn := func(ctx context.Context, key string) (string, error) {
		att := attempts.Add(1)
		if att == 1 {
			close(firstAttemptStarted)
			<-firstAttemptBlock
			return "", errors.New("transient database connection error")
		}
		return "successful-value", nil
	}

	var wg sync.WaitGroup
	results := make([]string, 5)
	errs := make([]error, 5)

	// Goroutine 0 triggers the first attempt which will fail
	wg.Add(1)
	goroutinelabels.NewGoroutine("resource_cache_test", "concurrent retry first attempt worker").StartSimple(func() {
		defer wg.Done()
		results[0], errs[0] = rc.GetOrCreate(ctx, "concurrent-key", initFn)
	})

	// Wait until goroutine 0 has entered initFn
	<-firstAttemptStarted

	// Launch 4 concurrent callers that join the in-flight initialization
	for i := 1; i < 5; i++ {
		idx := i
		wg.Add(1)
		goroutinelabels.NewGoroutine("resource_cache_test", "concurrent retry joint worker").StartSimple(func() {
			defer wg.Done()
			results[idx], errs[idx] = rc.GetOrCreate(ctx, "concurrent-key", initFn)
		})
	}

	// Small pause so concurrent callers join the wait on ready channel
	time.Sleep(20 * time.Millisecond)

	// Release goroutine 0 to fail
	close(firstAttemptBlock)
	wg.Wait()

	// Goroutine 0 must have received the transient error
	if errs[0] == nil {
		t.Fatalf("expected goroutine 0 to receive transient error, got val=%s", results[0])
	}

	// The remaining callers must have retried after goroutine 0 evicted the failed entry, and succeeded!
	for i := 1; i < 5; i++ {
		if errs[i] != nil {
			t.Fatalf("expected caller %d to retry and succeed, got err: %v", i, errs[i])
		}
		if results[i] != "successful-value" {
			t.Fatalf("expected caller %d to get successful-value, got %s", i, results[i])
		}
	}
}

func TestResourceCache_ContextCancellationDuringWait(t *testing.T) {
	t.Parallel()

	rc := &ResourceCache[string]{}

	started := make(chan struct{})
	block := make(chan struct{})
	defer close(block)

	goroutinelabels.NewGoroutine("resource_cache_test", "slow-key background worker").StartSimple(func() {
		if _, err := rc.GetOrCreate(context.Background(), "slow-key", func(ctx context.Context, key string) (string, error) {
			close(started)
			<-block
			return "slow-val", nil
		}); err != nil {
			t.Errorf("unexpected error in background GetOrCreate: %v", err)
		}
	})

	<-started

	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := rc.GetOrCreate(waitCtx, "slow-key", func(ctx context.Context, key string) (string, error) {
		return "unexpected", nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded error on cancelled context while waiting, got: %v", err)
	}
}
