package loader

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type testStateCallback struct {
	loading bool
	loaded  bool
	err     error
	timeout bool
}

func (c *testStateCallback) OnLoading()    { c.loading = true }
func (c *testStateCallback) OnLoaded(any)  { c.loaded = true }
func (c *testStateCallback) OnError(e error) { c.err = e }
func (c *testStateCallback) OnTimeout()    { c.timeout = true }

func TestLoadState_String(t *testing.T) {
	if LoadStateUnloaded.String() != "unloaded" {
		t.Fatalf("unexpected string: %s", LoadStateUnloaded.String())
	}
	if LoadStateLoading.String() != "loading" {
		t.Fatalf("unexpected string: %s", LoadStateLoading.String())
	}
	if LoadStateLoaded.String() != "loaded" {
		t.Fatalf("unexpected string: %s", LoadStateLoaded.String())
	}
	if LoadStateError.String() != "error" {
		t.Fatalf("unexpected string: %s", LoadStateError.String())
	}
	if LoadState(99).String() != "unknown" {
		t.Fatalf("unexpected string: %s", LoadState(99).String())
	}
}

func TestNoOpStateChangeCallback(t *testing.T) {
	var cb NoOpStateChangeCallback
	cb.OnLoading()
	cb.OnLoaded("val")
	cb.OnError(errors.New("err"))
	cb.OnTimeout()
}

func TestRunner_LifecycleAndConcurrency(t *testing.T) {
	cfg := LoaderTimeoutConfig{
		WaitForCompletion: 2 * time.Second,
		PublishChannel:    500 * time.Millisecond,
		LoadOperation:     2 * time.Second,
	}

	cb := &testStateCallback{}
	count := 0
	loadFn := func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		count++
		return nil
	}

	r := NewRunner("test_runner", loadFn, WithCallback(cb), WithTimeoutConfig(cfg))
	if r.TimeoutConfig() != cfg {
		t.Fatal("TimeoutConfig mismatch")
	}
	if r.Loaded() {
		t.Fatal("runner should not be loaded yet")
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.Load(ctx); err != nil {
				t.Errorf("concurrent Load failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if !r.Loaded() {
		t.Fatal("runner should be loaded")
	}
	if !cb.loading || !cb.loaded {
		t.Fatalf("callbacks not triggered: loading=%v loaded=%v", cb.loading, cb.loaded)
	}
	if count != 1 {
		t.Fatalf("expected count=1, got %d", count)
	}

	// Calling Load when already loaded should be no-op fast path
	if err := r.Load(ctx); err != nil {
		t.Fatalf("Load on loaded runner failed: %v", err)
	}

	// ResetLoaded
	r.ResetLoaded()
	if r.Loaded() {
		t.Fatal("runner should not be loaded after reset")
	}
}

func TestRunner_LoadError(t *testing.T) {
	testErr := errors.New("load failed")
	cb := &testStateCallback{}
	r := NewRunner("failing_runner", func(ctx context.Context) error {
		return testErr
	}, WithCallback(cb))

	err := r.Load(context.Background())
	if !errors.Is(err, testErr) {
		t.Fatalf("expected testErr, got %v", err)
	}
	if !cb.loading || cb.err == nil {
		t.Fatalf("expected OnError callback, got loading=%v err=%v", cb.loading, cb.err)
	}
}

func TestMergeLoaderTimeoutOverrides(t *testing.T) {
	base := DefaultLoaderTimeoutConfigMap()

	overrides := map[string]map[string]any{
		"custom_loader": {
			configKeyTimeoutSeconds:           10,
			configKeyWaitForCompletionSeconds: 15,
			configKeyPublishChannelSeconds:    2,
			configKeyLoadOperationSeconds:     5,
		},
	}

	merged := MergeLoaderTimeoutOverrides(base, overrides)
	c := merged["custom_loader"]
	if c.WaitForCompletion != 15*time.Second {
		t.Fatalf("got %v, want 15s", c.WaitForCompletion)
	}
	if c.PublishChannel != 2*time.Second {
		t.Fatalf("got %v, want 2s", c.PublishChannel)
	}
	if c.LoadOperation != 5*time.Second {
		t.Fatalf("got %v, want 5s", c.LoadOperation)
	}

	// Empty overrides returns base
	if m := MergeLoaderTimeoutOverrides(base, nil); len(m) != len(base) {
		t.Fatalf("expected base returned on empty overrides")
	}

	// intFromOverride helper
	if intFromOverride(nil) != 0 {
		t.Fatal("expected 0 for nil")
	}
	if intFromOverride(10) != 10 {
		t.Fatal("expected 10 for int")
	}
	if intFromOverride(float64(25.0)) != 25 {
		t.Fatal("expected 25 for float64")
	}
	if intFromOverride("string") != 0 {
		t.Fatal("expected 0 for string")
	}
}

func TestGetLoaderTimeoutConfig(t *testing.T) {
	cfg := GetLoaderTimeoutConfig("unknown_loader_name")
	def := DefaultLoaderTimeoutConfig()
	if cfg.WaitForCompletion != def.WaitForCompletion {
		t.Fatalf("expected default timeout config, got %+v", cfg)
	}
}
