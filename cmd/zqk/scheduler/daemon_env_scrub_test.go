package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestScrubDaemonInheritEnv_DropsParentAndMCPAccount(t *testing.T) {
	in := []string{
		"PATH=/bin",
		zqkenv.ParentPID().Name() + "=12345",
		zqkenv.MCPAccountID().Name() + "=test",
		zqkenv.IsParentZqk().Name() + "=0",
		"HOME=/tmp",
	}
	out := scrubDaemonInheritEnv(in)
	joined := stringsJoin(out)
	if containsEnvKey(out, zqkenv.ParentPID().Name()) {
		t.Fatalf("ParentPID still present: %v", out)
	}
	if containsEnvKey(out, zqkenv.MCPAccountID().Name()) {
		t.Fatalf("MCPAccountID still present: %v", out)
	}
	if !containsExact(out, zqkenv.IsParentZqk().Name() + "=1") {
		t.Fatalf("expected IsParentZqk=1 in %v", out)
	}
	if !containsExact(out, "PATH=/bin") || !containsExact(out, "HOME=/tmp") {
		t.Fatalf("lost unrelated env: %v (%s)", out, joined)
	}
}

func TestScrubSchedulerDaemonProcessEnv(t *testing.T) {
	t.Setenv(zqkenv.ParentPID().Name(), "999")
	t.Setenv(zqkenv.MCPAccountID().Name(), "test")
	scrubSchedulerDaemonProcessEnv()
	if zqkenv.ParentPID().Get() != "" {
		t.Fatal("ParentPID should be unset")
	}
	if zqkenv.MCPAccountID().Get() != "" {
		t.Fatal("MCPAccountID should be unset")
	}
	if zqkenv.IsParentZqk().Get() != "1" {
		t.Fatal("IsParentZqk should be 1")
	}
}

func containsEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func containsExact(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}

func stringsJoin(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}
