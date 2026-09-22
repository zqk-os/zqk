package ambient

import (
	"bytes"
	"strings"
	"testing"
)

func TestStartDaemon_refusesTestProcessAndCleansUp(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() { _ = StopDaemon(root) })

	var buf bytes.Buffer
	if err := StartDaemon(root, &buf); err != nil {
		t.Fatal(err)
	}
	if pid, ok := readAmbientPID(root); ok {
		t.Fatalf("test process must not Setsid-spawn ambient daemon, pid=%d", pid)
	}
	if got := buf.String(); !strings.Contains(got, "Skipping ambient daemon spawn") {
		t.Fatalf("want skip message, got %q", got)
	}
}

func TestEnsureDaemon_refusesTestProcess(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() { _ = StopDaemon(root) })
	if err := EnsureDaemon(root, nil); err != nil {
		t.Fatal(err)
	}
	if pid, ok := readAmbientPID(root); ok {
		t.Fatalf("EnsureDaemon must not orphan a test-binary daemon, pid=%d", pid)
	}
}
