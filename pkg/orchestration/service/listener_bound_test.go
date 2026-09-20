package service

import (
	"runtime"
	"testing"
)

func TestSynthesisIntentWorkersBounded(t *testing.T) {
	n := synthesisIntentWorkers()
	if n < 1 || n > 8 {
		t.Fatalf("synthesisIntentWorkers = %d, want 1..8 (GOMAXPROCS=%d)", n, runtime.GOMAXPROCS(0))
	}
}
