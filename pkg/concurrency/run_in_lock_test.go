package concurrency

import (
	"errors"
	"sync"
	"testing"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

var errRunInLockTest = errors.New("run_in_lock_test")

func TestRunInLock_SameGoroutine(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var touched bool
	err := RunInLock(&mu, func() error {
		touched = true
		return nil
	})
	if err != nil {
		t.Fatalf("RunInLock: %v", err)
	}
	if !touched {
		t.Fatal("callback did not run")
	}
	// Lock was released; we can acquire again
	func() {
		mu.Lock()
		defer mu.Unlock()
	}()
}

func TestRunInLock_ReturnsError(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	err := RunInLock(&mu, func() error {
		return errRunInLockTest
	})
	if err != errRunInLockTest {
		t.Fatalf("RunInLock: got %v", err)
	}
	func() {
		mu.Lock()
		defer mu.Unlock()
	}()
}

func TestRunInRLock_SameGoroutine(t *testing.T) {
	t.Parallel()
	var mu sync.RWMutex
	var value int
	err := RunInRLock(&mu, func() error {
		value = 1
		return nil
	})
	if err != nil {
		t.Fatalf("RunInRLock: %v", err)
	}
	if value != 1 {
		t.Fatalf("callback did not run: value=%d", value)
	}
	func() {
		mu.RLock()
		defer mu.RUnlock()
	}()
}

func TestRunInLock_Concurrent(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var counter int
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("concurrency_test", "run in lock concurrent").StartSimple(func() {
			defer wg.Done()
			_ = RunInLock(&mu, func() error {
				counter++
				return nil
			})
		})
	}
	wg.Wait()
	if counter != 10 {
		t.Fatalf("counter=%d", counter)
	}
}
