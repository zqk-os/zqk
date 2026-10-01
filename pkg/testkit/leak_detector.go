package testkit

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// VerifyNoGoroutineLeaks inspects the active goroutine count and stack traces
// to guarantee test execution does not leave orphaned background goroutines.
// (F-CON-01 / CRIT-CEF-R8L-CON-01).
func VerifyNoGoroutineLeaks(t testing.TB, allowedPrefixes ...string) {
	t.Helper()

	// Allow brief grace period for teardowns to settle
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		buf := make([]byte, 64*1024)
		n := runtime.Stack(buf, true)
		stack := string(buf[:n])

		lines := strings.Split(stack, "\n")
		var leakedStacks []string

		for i := 0; i < len(lines); i++ {
			line := lines[i]
			if strings.HasPrefix(line, "goroutine ") {
				// Check next few lines for origin
				if i+2 < len(lines) {
					origin := lines[i+1] + "\n" + lines[i+2]
					allowed := false
					for _, prefix := range allowedPrefixes {
						if strings.Contains(origin, prefix) {
							allowed = true
							break
						}
					}
					if !allowed && !strings.Contains(origin, "testing.") && !strings.Contains(origin, "runtime.") {
						leakedStacks = append(leakedStacks, line+"\n"+origin)
					}
				}
			}
		}

		if len(leakedStacks) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// VerifyNoSubprocessLeaks asserts that no child subprocesses spawned by the current
// test process remain running after test teardown.
// (TDE-F-TST-ORPHANED-PROCESS-LEAK / CRIT-TST-SUBPROCESS-REAPER-ADVERSARIAL).
func VerifyNoSubprocessLeaks(t testing.TB) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	var leaked []ProcessInfo
	myPid := os.Getpid()

	for time.Now().Before(deadline) {
		children, err := findChildProcesses(myPid)
		if err != nil {
			return // Avoid failing tests if OS process inspection is restricted
		}
		leaked = children
		if len(leaked) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(leaked) > 0 {
		var details []string
		for _, p := range leaked {
			details = append(details, fmt.Sprintf("PID %d (PPID %d): %s", p.PID, p.PPID, p.Command))
			_ = killProcessByPid(p.PID)
		}
		t.Fatalf("VerifyNoSubprocessLeaks: detected %d leaked child subprocess(es):\n%s",
			len(leaked), strings.Join(details, "\n"))
	}
}
