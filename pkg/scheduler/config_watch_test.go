package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

func newConfigWatchScheduler(t *testing.T, root string, running, onlyManual bool) *Scheduler {
	t.Helper()
	return &Scheduler{
		projectRoot:    root,
		running:        running,
		onlyManualJobs: onlyManual,
		logger:         logging.GetLoggerFromProfile("test"),
		jobs:           make(map[string]*ScheduledJob),
	}
}

func TestApplySchedulerConfigWatchTick_peekSkipsLoadWhenStampUnchanged(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := SaveSchedulerConfig(root, &SchedulerConfig{Enabled: true, JobsPaused: false}); err != nil {
		t.Fatalf("SaveSchedulerConfig: %v", err)
	}
	s := newConfigWatchScheduler(t, root, true, false)
	ctx := context.Background()

	first := s.applySchedulerConfigWatchTick(ctx)
	if !first.Loaded || first.PeekHit {
		t.Fatalf("first tick: got loaded=%v peek=%v, want loaded and not peek", first.Loaded, first.PeekHit)
	}
	configPath := schedulerConfigFilePath(root)
	if _, ok := schedulerConfigs.Peek(root, stampmemo.Of(configPath)); !ok {
		t.Fatal("schedulerConfigs should retain the first load")
	}

	second := s.applySchedulerConfigWatchTick(ctx)
	if !second.PeekHit || second.Loaded {
		t.Fatalf("second tick: got peek=%v loaded=%v, want peek hit and no Load", second.PeekHit, second.Loaded)
	}
	if second.ReloadRequested || second.Applied {
		t.Fatalf("second tick should be a no-op apply: %+v", second)
	}
}

func TestWatchSchedulerConfig_reloadRequestWakesUnder500ms(t *testing.T) {
	root := t.TempDir()
	if err := SaveSchedulerConfig(root, &SchedulerConfig{Enabled: true, JobsPaused: true}); err != nil {
		t.Fatalf("SaveSchedulerConfig: %v", err)
	}
	s := newConfigWatchScheduler(t, root, true, false)
	ready := make(chan struct{})
	s.TestHookConfigWatchReady = func() { close(ready) }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("scheduler_test", "watch scheduler config").StartSimple(func() {
		defer close(done)
		s.watchSchedulerConfig(ctx)
	})

	select {
	case <-ready:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("config watch did not arm")
	}

	start := time.Now()
	if err := WriteReloadConfigRequest(root); err != nil {
		t.Fatalf("WriteReloadConfigRequest: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for ReloadConfigRequestExists(root) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	elapsed := time.Since(start)
	if ReloadConfigRequestExists(root) {
		t.Fatalf("reload_config_request was not consumed within 500ms")
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("reload consume too slow: %s", elapsed)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchSchedulerConfig did not exit")
	}
	if !s.onlyManualJobs {
		t.Fatal("expected jobs_paused=true to apply after reload wake")
	}
}
