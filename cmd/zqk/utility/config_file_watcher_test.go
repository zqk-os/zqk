package utility

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestConfigFileWatcher_DefaultPollInterval_PreventsTickerPanic(t *testing.T) {
	t.Parallel()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-watcher-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	watcher := NewConfigFileWatcher(tmpDir, coordinator, logger)
	if watcher.pollInterval <= 0 {
		t.Fatalf("expected positive default pollInterval, got %v", watcher.pollInterval)
	}
	if watcher.pollInterval != DefaultConfigPollInterval {
		t.Errorf("expected pollInterval %v, got %v", DefaultConfigPollInterval, watcher.pollInterval)
	}

	// Setting non-positive duration should safely fall back to DefaultConfigPollInterval
	watcher.SetPollInterval(-1 * time.Second)
	if watcher.getPollInterval() <= 0 {
		t.Fatalf("expected positive fallback pollInterval, got %v", watcher.getPollInterval())
	}

	// Starting watcher and stopping it quickly should not panic
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start watcher failed: %v", err)
	}
	watcher.Stop()
}
