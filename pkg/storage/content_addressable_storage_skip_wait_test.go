package storage

import (
	"testing"
)

func TestCAS_SkipIndexUpdateWaitAtomic(t *testing.T) {
	t.Parallel()

	initVal := getSkipIndexUpdateWait()
	defer SetSkipIndexUpdateWait(initVal)

	SetSkipIndexUpdateWait(true)
	if !getSkipIndexUpdateWait() {
		t.Fatalf("expected getSkipIndexUpdateWait() to return true after Store(true)")
	}

	SetSkipIndexUpdateWait(false)
	if getSkipIndexUpdateWait() {
		t.Fatalf("expected getSkipIndexUpdateWait() to return false after Store(false)")
	}
}
