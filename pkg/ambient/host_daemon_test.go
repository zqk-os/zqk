package ambient

import "testing"

func TestHostDaemonEnabled_falseInGoTest(t *testing.T) {
	if HostDaemonEnabled(t.TempDir()) {
		t.Fatal("ambient host daemon must stay disabled inside go test")
	}
	if HostDaemonEnabled("") {
		t.Fatal("empty root must be disabled")
	}
}
