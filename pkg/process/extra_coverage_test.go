package process

import (
	"os"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestTimeoutMonitor_DisconnectAndReconnect(t *testing.T) {
	if IsTimeoutMonitorDisconnected() {
		t.Fatal("expected timeout monitor not to be disconnected initially")
	}

	exited1 := false
	exited2 := false

	reconnect := DisconnectTimeoutMonitor(func() {
		exited1 = true
	})

	if !IsTimeoutMonitorDisconnected() {
		t.Fatal("expected timeout monitor to be disconnected")
	}

	RegisterPagerExitCallback(func() {
		exited2 = true
	})
	RegisterPagerExitCallback(nil) // test nil callback tolerance

	// Call reconnect
	reconnect()

	if IsTimeoutMonitorDisconnected() {
		t.Fatal("expected timeout monitor to be reconnected")
	}
	if !exited1 || !exited2 {
		t.Fatalf("expected callbacks invoked: exited1=%v exited2=%v", exited1, exited2)
	}

	// Idempotent reconnect call
	reconnect()
}

func TestMeaningfulActivity(t *testing.T) {
	before := time.Now().Add(-time.Second)
	TouchMeaningfulActivity()
	last := GetLastMeaningfulActivity()

	if last.Before(before) {
		t.Fatalf("expected last activity %v to be after %v", last, before)
	}
}

func TestParentProcessName(t *testing.T) {
	// Negative and zero PID
	if got := ParentProcessName(0); got != "" {
		t.Fatalf("expected empty for pid 0, got %q", got)
	}
	if got := ParentProcessName(-1); got != "" {
		t.Fatalf("expected empty for negative pid, got %q", got)
	}

	// Current process PID
	name := ParentProcessName(os.Getpid())
	if name == "" {
		t.Fatal("expected non-empty name for current process")
	}

	// Non-existent PID
	_ = ParentProcessName(99999999)
}

func TestIsParentZqk(t *testing.T) {
	// Test env override
	zqkenv.IsParentZqk().Set("1")
	defer zqkenv.IsParentZqk().Unset()

	if !IsParentZqk() {
		t.Fatal("expected IsParentZqk to be true when env var is 1")
	}

	zqkenv.IsParentZqk().Unset()
	_ = IsParentZqk()
}

func TestOurExecutableBaseName(t *testing.T) {
	base := ourExecutableBaseName()
	if base == "" {
		t.Fatal("expected non-empty executable base name")
	}
}

func TestParentPID_Edges(t *testing.T) {
	if got := ParentPID(0); got != 0 {
		t.Fatalf("expected 0 for pid 0, got %d", got)
	}
	if got := ParentPID(-10); got != 0 {
		t.Fatalf("expected 0 for negative pid, got %d", got)
	}
	if got := ParentPID(99999999); got != 0 {
		t.Fatalf("expected 0 for nonexistent pid, got %d", got)
	}
}
