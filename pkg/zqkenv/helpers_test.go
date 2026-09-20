package zqkenv

import (
	"testing"
)

func TestApplyIsolatedStorageEnv_RequiresTestRootForFallthrough(t *testing.T) {
	t.Setenv(TestRoot().Name(), "")
	t.Setenv(TestAllowCASFallthrough().Name(), "")
	t.Setenv(PrivilegedWriterSocket().Name(), "")

	ApplyIsolatedStorageEnv(t.Setenv)
	if v := TestAllowCASFallthrough().Get(); v == enabledFlagValue {
		t.Fatalf("fallthrough must stay off when TEST_ROOT is empty; got %q", v)
	}
	if v := PrivilegedWriterSocket().Get(); v == "" {
		t.Fatal("expected privileged-writer socket override when isolation env applied")
	}

	t.Setenv(TestRoot().Name(), t.TempDir())
	t.Setenv(TestAllowCASFallthrough().Name(), "")
	ApplyIsolatedStorageEnv(t.Setenv)
	if v := TestAllowCASFallthrough().Get(); v != enabledFlagValue {
		t.Fatalf("fallthrough must enable when TEST_ROOT is set; got %q", v)
	}
}

func TestDaemonProcess(t *testing.T) {
	t.Setenv(IsDaemon().Name(), "")
	if DaemonProcess() {
		t.Fatal("expected DaemonProcess false when IS_DAEMON is unset")
	}
	t.Setenv(IsDaemon().Name(), enabledFlagValue)
	if !DaemonProcess() {
		t.Fatal("expected DaemonProcess true when IS_DAEMON=1")
	}
}
