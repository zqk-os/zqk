package audit

import "time"

const (
	AddEventLockAttempts = 10
	AddEventLockPause    = time.Millisecond
)

// TryAcquire calls tryLock up to attempts times, sleeping pause after each miss.
func TryAcquire(tryLock func() bool, attempts int, pause time.Duration) bool {
	if tryLock == nil || attempts <= 0 {
		return false
	}
	for i := 0; i < attempts; i++ {
		if tryLock() {
			return true
		}
		time.Sleep(pause)
	}
	return false
}
