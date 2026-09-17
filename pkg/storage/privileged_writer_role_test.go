package storage

import (
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func setPrivilegedWriterCommandArgs(t *testing.T, args []string) {
	t.Helper()
	orig := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = orig })
}

func TestRejectWriteBehindOnPrivilegedWriterRole_ArgvWithoutEnv(t *testing.T) {
	t.Setenv(zqkenv.IsDaemon().Name(), "")
	setPrivilegedWriterCommandArgs(t, []string{"zqk-stable", "object", "daemon"})

	if err := rejectWriteBehindOnPrivilegedWriterRole(&FileObjectStorage{}); err != nil {
		t.Fatalf("empty storage must pass: %v", err)
	}
	err := rejectWriteBehindOnPrivilegedWriterRole(&FileObjectStorage{
		writeBehindWorker: &ObjectWriteBehindWorker{},
	})
	if err == nil {
		t.Fatal("expected fail-closed when daemon argv owns write-behind")
	}
}

func TestRejectWriteBehindOnPrivilegedWriterRole_ClientArgv(t *testing.T) {
	t.Setenv(zqkenv.IsDaemon().Name(), "")
	setPrivilegedWriterCommandArgs(t, []string{"zqk", "object", "update", "CRIT-1"})

	err := rejectWriteBehindOnPrivilegedWriterRole(&FileObjectStorage{
		writeBehindWorker: &ObjectWriteBehindWorker{},
	})
	if err != nil {
		t.Fatalf("client argv may own write-behind: %v", err)
	}
}
