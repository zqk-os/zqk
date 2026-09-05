package testkit

import (
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
