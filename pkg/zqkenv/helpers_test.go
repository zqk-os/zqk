package zqkenv

import (
	"testing"
)

func TestApplyIsolatedStorageEnv_RequiresTestRootForFallthrough(t *testing.T) {
	t.Setenv(TestRoot(), "")
	t.Setenv(TestAllowCASFallthrough(), "")
	t.Setenv(PrivilegedWriterSocket(), "")

	ApplyIsolatedStorageEnv(t.Setenv)
	if v := Get(TestAllowCASFallthrough()).Val; v == enabledFlagValue {
		t.Fatalf("fallthrough must stay off when TEST_ROOT is empty; got %q", v)
	}
	if v := Get(PrivilegedWriterSocket()).Val; v == "" {
		t.Fatal("expected privileged-writer socket override when isolation env applied")
	}

	t.Setenv(TestRoot(), t.TempDir())
	t.Setenv(TestAllowCASFallthrough(), "")
	ApplyIsolatedStorageEnv(t.Setenv)
	if v := Get(TestAllowCASFallthrough()).Val; v != enabledFlagValue {
		t.Fatalf("fallthrough must enable when TEST_ROOT is set; got %q", v)
	}
}

func TestDaemonProcess(t *testing.T) {
	t.Setenv(IsDaemon(), "")
	if DaemonProcess() {
		t.Fatal("expected DaemonProcess false when IS_DAEMON is unset")
	}
	t.Setenv(IsDaemon(), enabledFlagValue)
	if !DaemonProcess() {
		t.Fatal("expected DaemonProcess true when IS_DAEMON=1")
	}
}
