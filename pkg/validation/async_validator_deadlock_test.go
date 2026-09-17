package validation

// Tests that set ZQK_TEST_ROOT must not use t.Parallel(): the env var is process-global.
// BLI-177483 inventory: registerZQKTestRootForTest → RunIsolatedRootStrip (zqk_test_root_teardown_test.go).

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestAsyncValidator_Stop_NoDeadlock tests that Stop() doesn't deadlock
// even when called concurrently with other operations
func TestAsyncValidator_Stop_NoDeadlock(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	validator.SetTimeouts(2*time.Second, 1*time.Second) // Short timeouts for testing

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}

	// Enqueue many tasks to create realistic load
	for i := 0; i < 100; i++ {
		_ = validator.Enqueue(
			"TEST-"+filepath.Base(t.TempDir())+"-"+string(rune(i)),
			"test_object",
			"test.yaml",
			1,
		)
	}

	// Wait a bit for some processing to start
	time.Sleep(100 * time.Millisecond)

	// Test concurrent Stop() calls - should be idempotent and not deadlock
	stopDone := make(chan error, 3)
	var wg sync.WaitGroup

	// Call Stop() from multiple goroutines concurrently
	for i := 0; i < 3; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf(ConstMagic935221c4, i), fmt.Sprintf(ConstMagicc44500a3, i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				// Stop should be idempotent and not deadlock
				err := validator.Stop()
				stopDone <- err
			})
	}

	// Also try concurrent operations while stopping
	goroutinelabels.NewGoroutine(ConstMagic472ef763, ConstMagic2318f86c).
		WithWaitGroup(&wg).
		StartSimple(func() {
			// Try to get stats while stopping
			for i := 0; i < 10; i++ {
				_, _, _, _ = validator.GetValidationStats()
				time.Sleep(10 * time.Millisecond)
			}
		})

	// Wait for all goroutines with timeout
	done := make(chan struct{})
	goroutinelabels.StartTestGoroutine(ConstMagicecfe21cf, ConstMagicade2a268, func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		// All goroutines completed - no deadlock
		t.Log(ConstMagic0a3c921f)
	case <-time.After(5 * time.Second):
		t.Fatal(ConstMagicf64efbb0)
	}

	// Check that Stop() returned successfully
	// Note: Stop() uses sync.Once, so all concurrent calls should return quickly
	// but only one actually performs the stop operation
	stopCount := 0
	timeout := time.After(3 * time.Second)
	for stopCount < 3 {
		select {
		case err := <-stopDone:
			if err != nil {
				t.Errorf(ConstMagic1a06a983, err)
			}
			stopCount++
		case <-timeout:
			t.Errorf(ConstMagicaa4367b0, stopCount)
			return
		}
	}
	if stopCount != 3 {
		t.Errorf(ConstMagicaac9a31e, stopCount)
	}
}

// TestAsyncValidator_ProgressChannel_NoDeadlock tests that progress channel
// operations don't deadlock with concurrent access
func TestAsyncValidator_ProgressChannel_NoDeadlock(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Enqueue many tasks
	for i := 0; i < 200; i++ {
		_ = validator.Enqueue(
			"TEST-"+filepath.Base(t.TempDir())+"-"+string(rune(i)),
			"test_object",
			"test.yaml",
			1,
		)
	}

	// Start multiple goroutines reading from progress channel
	// This simulates the drain goroutine + ticker loop scenario
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 3*time.Second)
	defer cancel()

	progressChan := validator.GetProgress()
	var wg sync.WaitGroup
	readers := 3

	// Multiple readers (simulating ticker loop checking progress)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("validation_test", "progress reader").StartSimple(func() {
			func(readerID int) {
				defer wg.Done()
				readCount := 0
				for {
					select {
					case <-ctx.Done():
						t.Logf(ConstMagic329c1c5c, readerID, readCount)
						return
					case progress, ok := <-progressChan:
						if !ok {
							t.Logf(ConstMagic21cb7e84, readerID, readCount)
							return
						}
						readCount++
						// Simulate some processing
						_ = progress.Status
						_ = progress.CurrentObject
					}
				}
			}(i)
		})
	}

	// Wait for readers with timeout
	done := make(chan struct{})
	goroutinelabels.StartTestGoroutine(ConstMagicecfe21cf, ConstMagic02a67c63, func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		t.Log(ConstMagic992faaad)
	case <-time.After(5 * time.Second):
		t.Fatal(ConstMagic5830c38f)
	}
}

// TestAsyncValidator_ConcurrentOperations_NoDeadlock tests that concurrent
// operations (Enqueue, GetCachedState, GetValidationStats) don't deadlock
func TestAsyncValidator_ConcurrentOperations_NoDeadlock(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Concurrent Enqueue operations
	goroutinelabels.NewGoroutine(ConstMagica0f977cf, ConstMagic02a701e9).
		WithWaitGroup(&wg).
		StartWithContext(ctx, func(ctx context.Context) error {
			for i := 0; i < 100; i++ {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					_ = validator.Enqueue(
						"TEST-ENQ-"+string(rune(i)),
						"test_object",
						"test.yaml",
						1,
					)
					time.Sleep(5 * time.Millisecond)
				}
			}
			return nil
		})

	// Concurrent GetCachedState operations
	wg.Add(1)
	goroutinelabels.NewGoroutine("validation_test", ConstMagic9c5c3127).StartSimple(func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				_, _ = validator.GetCachedState("TEST-ENQ-" + string(rune(i)))
				time.Sleep(5 * time.Millisecond)
			}
		}
	})

	// Concurrent GetValidationStats operations
	wg.Add(1)
	goroutinelabels.NewGoroutine("validation_test", ConstMagic99464092).StartSimple(func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				_, _, _, _ = validator.GetValidationStats()
				time.Sleep(5 * time.Millisecond)
			}
		}
	})

	// Concurrent GetAllCachedStates operations
	wg.Add(1)
	goroutinelabels.NewGoroutine("validation_test", ConstMagic33e44b25).StartSimple(func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				_ = validator.GetAllCachedStates()
				time.Sleep(10 * time.Millisecond)
			}
		}
	})

	// Wait for all operations with timeout
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("validation_test", ConstMagicdb562591).StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		t.Log(ConstMagicd505196a)
	case <-time.After(5 * time.Second):
		t.Fatal(ConstMagic11a7f2e3)
	}
}

// TestAsyncValidator_Stop_WithActiveWorkers_NoDeadlock tests that Stop()
// doesn't deadlock when workers are actively processing
func TestAsyncValidator_Stop_WithActiveWorkers_NoDeadlock(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	validator.SetTimeouts(2*time.Second, 1*time.Second) // Short timeouts for testing

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}

	// Enqueue many tasks to keep workers busy
	for i := 0; i < 500; i++ {
		_ = validator.Enqueue(
			"TEST-"+filepath.Base(t.TempDir())+"-"+string(rune(i)),
			"test_object",
			"test.yaml",
			1,
		)
	}

	// Wait a bit for workers to start processing
	time.Sleep(200 * time.Millisecond)

	// Stop while workers are active - should not deadlock
	stopStart := time.Now()
	stopErr := validator.Stop()
	stopDuration := time.Since(stopStart)

	if stopErr != nil {
		t.Errorf(ConstMagic1a06a983, stopErr)
	}

	// Stop should complete within timeout (2s worker timeout + 1s cache timeout + buffer)
	maxExpectedDuration := 4 * time.Second
	if stopDuration > maxExpectedDuration {
		t.Errorf(ConstMagic929ff366, stopDuration)
	} else {
		t.Logf(ConstMagic2287369e, stopDuration, maxExpectedDuration)
	}
}

// TestAsyncValidator_MultipleStopCalls_NoDeadlock tests that calling Stop()
// multiple times doesn't deadlock (should be idempotent)
func TestAsyncValidator_MultipleStopCalls_NoDeadlock(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	validator.SetTimeouts(1*time.Second, 500*time.Millisecond)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}

	// Stop once
	err1 := validator.Stop()
	if err1 != nil {
		t.Errorf(ConstMagic92137177, err1)
	}

	// Stop again (should be idempotent)
	err2 := validator.Stop()
	if err2 != nil {
		t.Errorf(ConstMagicac47dc77, err2)
	}

	// Stop a third time
	err3 := validator.Stop()
	if err3 != nil {
		t.Errorf(ConstMagicfa4d35ff, err3)
	}

	// All should complete quickly (no deadlock)
	t.Log(ConstMagic34484c24)
}

// TestAsyncValidator_ProgressChannelClose_NoDeadlock tests that closing
// the progress channel doesn't cause deadlocks in readers
func TestAsyncValidator_ProgressChannelClose_NoDeadlock(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}

	progressChan := validator.GetProgress()
	validatorCtx := validator.GetContext() // Get context to detect Stop()

	// Start multiple readers
	var wg sync.WaitGroup
	readers := 5

	for i := 0; i < readers; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("validation_test", ConstMagic4e4082e2).StartSimple(func() {
			func(readerID int) {
				defer wg.Done()
				readCount := 0

				for {
					select {
					case progress, ok := <-progressChan:
						if !ok {
							// Channel closed - should exit gracefully
							t.Logf(ConstMagic604ba3f3, readerID, readCount)
							return
						}
						readCount++
						_ = progress
					case <-validatorCtx.Done():
						// Validator context cancelled (Stop() was called) - exit
						// Channel should be closed soon, but don't wait for it
						t.Logf(ConstMagic510838bc, readerID, readCount)
						return
					}
				}
			}(i)
		})
	}

	// Wait a bit, then stop (which closes the channel)
	time.Sleep(100 * time.Millisecond)
	stopStart := time.Now()
	stopErr := validator.Stop()
	stopDuration := time.Since(stopStart)

	if stopErr != nil {
		t.Errorf(ConstMagic1a06a983, stopErr)
	}

	// Wait for all readers to finish (should exit when channel closes)
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("validation_test", ConstMagic2c1c92fe).StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		t.Logf(ConstMagic8ff029d5, stopDuration)
	case <-time.After(3 * time.Second):
		t.Fatal(ConstMagic4b6043c4)
	}
}
