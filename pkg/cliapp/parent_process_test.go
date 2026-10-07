package cli

import (
	"os"
	"testing"
)

func TestParentProcessHelpers(t *testing.T) {
	name := ParentProcessName(os.Getppid())
	if name != "" {
		t.Logf("parent process name: %s", name)
	}
	isZqk := IsParentZqk()
	if isZqk {
		t.Log("parent process is zqk")
	}

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
