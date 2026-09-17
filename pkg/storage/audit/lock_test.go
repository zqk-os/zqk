package audit

import (
	"testing"
	"time"
)

func TestTryAcquire(t *testing.T) {
	t.Parallel()
	n := 0
	ok := TryAcquire(func() bool {
		n++
		return n >= 3
	}, 10, time.Nanosecond)
	if !ok || n != 3 {
		t.Fatalf("ok=%v n=%d", ok, n)
	}
	if TryAcquire(func() bool { return false }, 2, time.Nanosecond) {
		t.Fatal("expected miss")
	}
	if TryAcquire(nil, 3, 0) {
		t.Fatal("nil tryLock")
	}
}
