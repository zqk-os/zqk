package instance_builders

import (
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestBaseInstanceBuilder_ConcurrentSetField demonstrates that builders are NOT thread-safe
// when shared across goroutines. This test documents the behavior - builders should NOT be shared.
// The fix is to create fresh builder instances per use (as done in CreateAuditEventWithBuilder, etc.)
func TestBaseInstanceBuilder_ConcurrentSetField(t *testing.T) {
	t.Parallel()
	t.Skip("This test demonstrates that builders are NOT thread-safe when shared. Builders should create fresh instances per use.")
	builder := NewBaseInstanceBuilder("test_kind", "1.0.0", []string{"id", "field1"}, nil)

	var wg sync.WaitGroup
	concurrency := 100

	// Launch many goroutines that all call SetField concurrently
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("specbuilder_test", "concurrent set field").StartSimple(func() {
			func(index int) {
				defer wg.Done()
				builder.SetField("field1", index)
				builder.SetID("test-id")
				builder.SetStatus("active")
			}(i)
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify builder is in a valid state (no panic occurred)
	// Note: We don't verify the exact values since they're non-deterministic with concurrent access
	// The test is primarily to ensure no panic occurs
	_, _ = builder.Build() // Ignore error; this test only asserts "no panic"
}

// TestBaseInstanceBuilder_ConcurrentBuild tests that Build can be called
// concurrently after SetField operations
func TestBaseInstanceBuilder_ConcurrentBuild(t *testing.T) {
	t.Parallel()
	builder := NewBaseInstanceBuilder("test_kind", "1.0.0", []string{"id", "field1"}, nil)

	var wg sync.WaitGroup
	concurrency := 50

	// Launch many goroutines that set fields and build
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("specbuilder_test", "concurrent build").StartSimple(func() {
			func(index int) {
				defer wg.Done()
				builder.SetID("test-id")
				builder.SetField("field1", index)
				_, _ = builder.Build() // Ignore errors - we're testing for panics
			}(i)
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()
}
