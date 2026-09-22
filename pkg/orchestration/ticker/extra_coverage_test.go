// BLI-STARTER-COMMUNITY-041 / PRI-STARTER-COMMUNITY-041 coverage elevation
package ticker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/orchestration"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestExtraTickerDisplayAndHeartbeat(t *testing.T) {
	root := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), root)
	t.Setenv(zqkenv.TestRoot().Name(), root)

	tk := NewActivityTicker()
	tk.Display(time.Millisecond)

	logDir := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir)
	if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	tk.Tick("coder", "lane-a")
	tk.Display(50 * time.Millisecond)
	logPath := filepath.Join(logDir, orchestration.LogKeyHiveActivity)
	if !fileutil.Exists(logPath) {
		t.Fatalf("missing ticker log %s", logPath)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tk.RunHeartbeat(ctx, 15*time.Millisecond)
		close(done)
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat did not stop")
	}
}
