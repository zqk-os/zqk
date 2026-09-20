package testkit

import (
	"testing"
)

func TestVerifyNoGoroutineLeaks(t *testing.T) {
	// Baseline clean test run
	VerifyNoGoroutineLeaks(t)
}
