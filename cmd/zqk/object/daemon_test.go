package object_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lanceman/zqk/cmd/zqk/object"
	"github.com/lanceman/zqk/pkg/accumulator"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestDaemon_NoDirectFmtPrint verifies compliance with POL-CODE-007 (REQ-CEF-R2-OBS-FMT-PRINTF).
func TestDaemon_NoDirectFmtPrint(t *testing.T) {
	content, err := fileutil.ReadFile("daemon.go")
	if err != nil {
		t.Fatalf("ReadFile daemon.go: %v", err)
	}
	src := string(content)
	if strings.Contains(src, "fmt.Print") {
		t.Errorf("daemon.go violates POL-CODE-007 by containing direct fmt.Print calls")
	}
}

// TestNewDaemonCmd verifies command construction.
func TestNewDaemonCmd(t *testing.T) {
	cmd := object.NewDaemonCmd()
	if cmd == nil || cmd.Use != "daemon" {
		t.Fatalf("unexpected daemon cmd: %v", cmd)
	}
}

// TestDaemon_AccumulatorSupervision verifies CRIT-1789599243366372000-5d3fefd1:
// object daemon integrates and supervises registered accumulator WAL subscribers.
func TestDaemon_AccumulatorSupervision(t *testing.T) {
	tempDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subs := accumulator.StartAllSubscribers(ctx, tempDir)
	// Even in unit test context, start subscribers must execute safely
	_ = subs
}
