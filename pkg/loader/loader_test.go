package loader

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/kindnames"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockCallback struct {
	loadingCount atomic.Int32
	loadedCount  atomic.Int32
	errorCount   atomic.Int32
	timeoutCount atomic.Int32
	lastErr      atomic.Pointer[error]
	lastVal      atomic.Pointer[any]
}

func (m *mockCallback) OnLoading() {
	m.loadingCount.Add(1)
}

func (m *mockCallback) OnLoaded(val any) {
	m.loadedCount.Add(1)
	m.lastVal.Store(&val)
}

func (m *mockCallback) OnError(err error) {
	m.errorCount.Add(1)
	m.lastErr.Store(&err)
}

func (m *mockCallback) OnTimeout() {
	m.timeoutCount.Add(1)
}

func TestLoaderDefaultsAndRegistryRetrieval(t *testing.T) {
	// Test DefaultLoaderTimeoutConfig
	def := DefaultLoaderTimeoutConfig()
	if def.WaitForCompletion != 5*time.Second {
		t.Fatalf("expected WaitForCompletion 5s, got %v", def.WaitForCompletion)
	}
	if def.PublishChannel != 1*time.Second {
		t.Fatalf("expected PublishChannel 1s, got %v", def.PublishChannel)
	}
	if def.LoadOperation != 0 {
		t.Fatalf("expected LoadOperation 0, got %v", def.LoadOperation)
	}

	// Test DefaultLoaderTimeoutConfigMap
	cfgMap := DefaultLoaderTimeoutConfigMap()
	expectedLoaders := []string{
		"id_patterns",
		kindnames.Lifecycle,
		"bucketing_config",
		"bucket_strategy_loader",
		"spec_loader",
		"kind_synonym_resolver",
		"kind_mapper",
		"object_id_cache",
	}

	for _, name := range expectedLoaders {
		cfg, exists := cfgMap[name]
		if !exists {
			t.Errorf("expected loader %q in default timeout config map", name)
		}
		if cfg.WaitForCompletion <= 0 {
			t.Errorf("loader %q expected positive WaitForCompletion, got %v", name, cfg.WaitForCompletion)
		}
		if cfg.PublishChannel <= 0 {
			t.Errorf("loader %q expected positive PublishChannel, got %v", name, cfg.PublishChannel)
		}
	}

	// Known loader vs Unknown loader via GetLoaderTimeoutConfig
	knownCfg := GetLoaderTimeoutConfig("id_patterns")
	if knownCfg.WaitForCompletion != 15*time.Second {
		t.Fatalf("expected 15s for id_patterns, got %v", knownCfg.WaitForCompletion)
	}

	unknownCfg := GetLoaderTimeoutConfig("nonexistent_loader_key")
	if unknownCfg.WaitForCompletion != def.WaitForCompletion {
		t.Fatalf("expected default config for unknown loader, got %v", unknownCfg)
	}

	// GetGlobalLoaderTimeoutConfigMap
	globalMap := GetGlobalLoaderTimeoutConfigMap()
	if len(globalMap) == 0 {
		t.Fatal("expected non-empty global loader timeout config map")
	}
}

func TestLoadLoaderTimeoutConfig_PathsAndErrors(t *testing.T) {
	// 1. Empty string fallback to findLoaderConfigFile
	m, err := LoadLoaderTimeoutConfig("")
	if err != nil {
		t.Fatalf("expected nil error on empty path, got %v", err)
	}
	if len(m) == 0 {
		t.Fatal("expected non-empty config map for empty path")
	}

	// 2. Non-existent file path returns defaults
	nonExistent := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	mNonExistent, err := LoadLoaderTimeoutConfig(nonExistent)
	if err != nil {
		t.Fatalf("expected nil error for non-existent file, got %v", err)
	}
	if mNonExistent["id_patterns"].WaitForCompletion != 15*time.Second {
		t.Fatalf("expected default id_patterns config, got %v", mNonExistent["id_patterns"])
	}

	// 3. Invalid YAML returns defaults
	invalidYAML := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := fileutil.WriteFile(invalidYAML, []byte("component_loaders: [invalid yaml syntax"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	mInvalid, err := LoadLoaderTimeoutConfig(invalidYAML)
	if err != nil {
		t.Fatalf("expected nil error for invalid YAML file, got %v", err)
	}
	if mInvalid["id_patterns"].WaitForCompletion != 15*time.Second {
		t.Fatalf("expected default config fallback on invalid YAML, got %v", mInvalid["id_patterns"])
	}

	// 4. Valid YAML with TimeoutSeconds shorthand
	validShorthand := filepath.Join(t.TempDir(), "shorthand.yaml")
	yamlContent := []byte(`
component_loaders:
  test_loader:
    timeout_seconds: 42
`)
	if err := fileutil.WriteFile(validShorthand, yamlContent, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	mShorthand, err := LoadLoaderTimeoutConfig(validShorthand)
	if err != nil {
		t.Fatalf("expected nil error for valid YAML, got %v", err)
	}
	tl := mShorthand["test_loader"]
	if tl.WaitForCompletion != 42*time.Second || tl.LoadOperation != 42*time.Second {
		t.Fatalf("expected 42s for wait and load, got wait=%v load=%v", tl.WaitForCompletion, tl.LoadOperation)
	}

	// 5. Valid YAML with distinct fields
	validDistinct := filepath.Join(t.TempDir(), "distinct.yaml")
	distinctContent := []byte(`
component_loaders:
  distinct_loader:
    wait_for_completion_seconds: 12
    publish_channel_seconds: 3
    load_operation_seconds: 7
`)
	if err := fileutil.WriteFile(validDistinct, distinctContent, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	mDistinct, err := LoadLoaderTimeoutConfig(validDistinct)
	if err != nil {
		t.Fatalf("expected nil error for valid YAML, got %v", err)
	}
	dl := mDistinct["distinct_loader"]
	if dl.WaitForCompletion != 12*time.Second || dl.PublishChannel != 3*time.Second || dl.LoadOperation != 7*time.Second {
		t.Fatalf("expected 12s/3s/7s, got wait=%v pub=%v load=%v", dl.WaitForCompletion, dl.PublishChannel, dl.LoadOperation)
	}
}

func TestRunner_OptionsAndNilCallback(t *testing.T) {
	// WithCallback(nil) should not overwrite the default NoOpStateChangeCallback
	r := NewRunner("test_nil_cb", func(ctx context.Context) error { return nil }, WithCallback(nil))
	if r.callback == nil {
		t.Fatal("expected non-nil default callback when WithCallback(nil) is passed")
	}

	// Load should execute smoothly with NoOp callback
	if err := r.Load(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Loaded() {
		t.Fatal("expected runner to be loaded")
	}
}

func TestRunner_LoadOperationTimeout(t *testing.T) {
	cfg := LoaderTimeoutConfig{
		WaitForCompletion: 500 * time.Millisecond,
		PublishChannel:    100 * time.Millisecond,
		LoadOperation:     30 * time.Millisecond,
	}

	var loadCtxErr error
	loadFn := func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			loadCtxErr = ctx.Err()
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return nil
		}
	}

	cb := &mockCallback{}
	r := NewRunner("timeout_op_runner", loadFn, WithTimeoutConfig(cfg), WithCallback(cb))

	err := r.Load(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if !errors.Is(loadCtxErr, context.DeadlineExceeded) {
		t.Fatalf("expected loadCtxErr to be DeadlineExceeded, got %v", loadCtxErr)
	}
	if cb.errorCount.Load() != 1 {
		t.Fatalf("expected OnError callback called once, got %d", cb.errorCount.Load())
	}
	if r.Loaded() {
		t.Fatal("runner should not be marked loaded after failure")
	}
}

func TestRunner_WaitForCompletionTimeout(t *testing.T) {
	cfg := LoaderTimeoutConfig{
		WaitForCompletion: 40 * time.Millisecond,
		PublishChannel:    100 * time.Millisecond,
		LoadOperation:     500 * time.Millisecond,
	}

	started := make(chan struct{})
	release := make(chan struct{})

	loadFn := func(ctx context.Context) error {
		close(started)
		<-release
		return nil
	}

	cb := &mockCallback{}
	r := NewRunner("wait_timeout_runner", loadFn, WithTimeoutConfig(cfg), WithCallback(cb))

	// Goroutine 1 starts load
	goroutinelabels.NewGoroutine("test_runner_wait_timeout", "starts long load").StartSimple(func() {
		_ = r.Load(context.Background())
	})

	<-started

	// Goroutine 2 enters waitForCompletion and times out
	err := r.Load(context.Background())
	if err == nil {
		t.Fatal("expected timeout error for waiter, got nil")
	}
	expectedMsg := fmt.Sprintf(errWaitLoadTimeoutFmt, cfg.WaitForCompletion)
	if err.Error() != expectedMsg {
		t.Fatalf("expected %q, got %q", expectedMsg, err.Error())
	}
	if cb.timeoutCount.Load() != 1 {
		t.Fatalf("expected OnTimeout called once, got %d", cb.timeoutCount.Load())
	}

	// Clean up goroutine 1
	close(release)
}

func TestRunner_WaitForCompletion_ContextCancelled(t *testing.T) {
	cfg := LoaderTimeoutConfig{
		WaitForCompletion: 500 * time.Millisecond,
		PublishChannel:    100 * time.Millisecond,
	}

	started := make(chan struct{})
	release := make(chan struct{})

	loadFn := func(ctx context.Context) error {
		close(started)
		<-release
		return nil
	}

	r := NewRunner("wait_cancel_runner", loadFn, WithTimeoutConfig(cfg))

	goroutinelabels.NewGoroutine("test_runner_wait_cancel", "starts load").StartSimple(func() {
		_ = r.Load(context.Background())
	})

	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := r.Load(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	close(release)
}

func TestRunner_WaitForCompletion_PublishChannelTimeout(t *testing.T) {
	cfg := LoaderTimeoutConfig{
		WaitForCompletion: 500 * time.Millisecond,
		PublishChannel:    25 * time.Millisecond,
	}

	r := NewRunner("pub_timeout_runner", func(ctx context.Context) error { return nil }, WithTimeoutConfig(cfg))

	// Manually set loading to true without storing a loadDone channel
	r.loading.Store(true)

	err := r.waitForCompletion(context.Background())
	if err == nil {
		t.Fatal("expected channel publish timeout error, got nil")
	}
	expectedMsg := fmt.Sprintf(errWaitChannelTimeoutFmt, cfg.PublishChannel)
	if err.Error() != expectedMsg {
		t.Fatalf("expected %q, got %q", expectedMsg, err.Error())
	}

	// If loaded becomes true while waiting for channel
	r.loaded.Store(true)
	err = r.waitForCompletion(context.Background())
	if err != nil {
		t.Fatalf("expected nil when loaded is true, got %v", err)
	}

	// Context cancellation while polling for publish channel
	r.loaded.Store(false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = r.waitForCompletion(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled while polling publish channel, got %v", err)
	}
}

func TestRunner_WaitForCompletion_PostLoopLoaded(t *testing.T) {
	// If PublishChannel has elapsed immediately and loaded is true, return nil (line 100-101)
	cfg := LoaderTimeoutConfig{
		PublishChannel: -1 * time.Second,
	}
	r := NewRunner("post_loop_runner", func(ctx context.Context) error { return nil }, WithTimeoutConfig(cfg))
	r.loading.Store(true)
	r.loaded.Store(true)

	err := r.waitForCompletion(context.Background())
	if err != nil {
		t.Fatalf("expected nil when loaded is true post-loop, got %v", err)
	}
}

func TestRunner_WaitForCompletion_ChannelClosedWithNoErrAndNotLoaded(t *testing.T) {
	// If ch closes, loaded is false, and lastErr is nil, return nil (line 113)
	cfg := LoaderTimeoutConfig{
		WaitForCompletion: 500 * time.Millisecond,
		PublishChannel:    100 * time.Millisecond,
	}
	r := NewRunner("closed_ch_runner", func(ctx context.Context) error { return nil }, WithTimeoutConfig(cfg))
	ch := make(chan struct{})
	close(ch)
	r.loadDone.Store(ch)
	r.loading.Store(true)
	r.loaded.Store(false)

	err := r.waitForCompletion(context.Background())
	if err != nil {
		t.Fatalf("expected nil when channel closed with nil lastErr, got %v", err)
	}
}

func TestRunner_WaitForCompletion_PropagatesLoadErrorToWaiters(t *testing.T) {
	cfg := LoaderTimeoutConfig{
		WaitForCompletion: 500 * time.Millisecond,
		PublishChannel:    100 * time.Millisecond,
	}

	started := make(chan struct{})
	release := make(chan struct{})
	expectedErr := errors.New("underlying load failed")

	loadFn := func(ctx context.Context) error {
		close(started)
		<-release
		return expectedErr
	}

	cb := &mockCallback{}
	r := NewRunner("error_prop_runner", loadFn, WithTimeoutConfig(cfg), WithCallback(cb))

	var waitErr error
	var wg sync.WaitGroup
	wg.Add(1)

	// Goroutine 1 starts load
	goroutinelabels.NewGoroutine("test_error_prop_loader", "runs load function").StartSimple(func() {
		_ = r.Load(context.Background())
	})

	<-started

	// Goroutine 2 waits for completion
	goroutinelabels.NewGoroutine("test_error_prop_waiter", "waits for completion").StartSimple(func() {
		defer wg.Done()
		waitErr = r.Load(context.Background())
	})

	// Small pause so goroutine 2 is waiting on channel
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if !errors.Is(waitErr, expectedErr) {
		t.Fatalf("expected waiter to receive expectedErr, got %v", waitErr)
	}
	if cb.errorCount.Load() != 1 {
		t.Fatalf("expected error count 1, got %d", cb.errorCount.Load())
	}
}

func TestRunner_CASContention(t *testing.T) {
	// Verify that multiple goroutines contending for loading CAS cleanly resolve
	for round := 0; round < 30; round++ {
		loadCount := atomic.Int32{}
		loadFn := func(ctx context.Context) error {
			loadCount.Add(1)
			time.Sleep(2 * time.Millisecond)
			return nil
		}

		r := NewRunner("cas_contention_runner", loadFn)

		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 100; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine(fmt.Sprintf("test_cas_contender_%d", i), "contending load caller").StartSimple(func() {
				defer wg.Done()
				<-start
				if err := r.Load(context.Background()); err != nil {
					t.Errorf("unexpected error in load: %v", err)
				}
			})
		}
		close(start)
		wg.Wait()

		if loadCount.Load() != 1 {
			t.Fatalf("expected exactly 1 load execution, got %d", loadCount.Load())
		}
		if !r.Loaded() {
			t.Fatal("expected runner to be loaded")
		}
	}
}
