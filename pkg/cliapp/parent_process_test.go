package cli

import (
	"os"
	"testing"
)

func TestParentProcessHelpers(t *testing.T) {
	_ = ParentProcessName(os.Getppid())
	_ = IsParentZqk()

	TouchMeaningfulActivity()
	last := GetLastMeaningfulActivity()
	if last.IsZero() {
		t.Errorf("expected non-zero last meaningful activity timestamp")
	}

	reconnect := DisconnectTimeoutMonitor()
	if !IsTimeoutMonitorDisconnected() {
		t.Errorf("expected timeout monitor to report disconnected")
	}
	reconnect()

	RegisterPagerExitCallback(func() {})
}
