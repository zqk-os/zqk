package mcp

import (
	"testing"
	"time"
)

// waitForNSignals waits for n receives from ch, or fails the test on timeout.
// Use when multiple goroutines each send one value to the same channel.
func waitForNSignals(t *testing.T, ch <-chan bool, n int, timeout time.Duration, timeoutMsg string) {
	t.Helper()
	deadline := time.After(timeout)
	completed := 0
	for completed < n {
		select {
		case <-ch:
			completed++
		case <-deadline:
			t.Fatal(timeoutMsg)
		}
	}
}

// waitForOneFromEach waits for one receive from each of ch1 and ch2 (in any order), or fails on timeout.
// Use when two goroutines each signal a different channel.
func waitForOneFromEach(t *testing.T, ch1, ch2 <-chan bool, timeout time.Duration, timeoutMsg string) {
	t.Helper()
	deadline := time.After(timeout)
	got1, got2 := false, false
	for !got1 || !got2 {
		select {
		case <-ch1:
			got1 = true
		case <-ch2:
			got2 = true
		case <-deadline:
			t.Fatal(timeoutMsg)
		}
	}
}

// waitForSignal waits for one receive from ch, or fails the test on timeout.
func waitForSignal(t *testing.T, ch <-chan bool, timeout time.Duration, timeoutMsg string) {
	t.Helper()
	select {
	case <-ch:
		return
	case <-time.After(timeout):
		t.Fatal(timeoutMsg)
	}
}
