package goroutinelabels

import (
	"fmt"
	"sync"
	"testing"
)

// TestGoroutineBuilder provides a simplified builder for test code
// It automatically handles common test patterns like wait groups
type TestGoroutineBuilder struct {
	*GoroutineBuilder
	wg *sync.WaitGroup
}

// NewTestGoroutine creates a new test goroutine builder
// Test goroutines automatically get a wait group for coordination
func NewTestGoroutine(name, purpose string) *TestGoroutineBuilder {
	wg := &sync.WaitGroup{}
	return &TestGoroutineBuilder{
		GoroutineBuilder: NewGoroutine(name, purpose),
		wg:               wg,
	}
}

// Start starts the goroutine and automatically adds it to the wait group
// Note: Add(1) is now called automatically by StartSimple when WithWaitGroup is set
func (tb *TestGoroutineBuilder) Start(fn func()) {
	tb.GoroutineBuilder.WithWaitGroup(tb.wg).StartSimple(fn)
}

// Wait waits for all goroutines started with this builder to complete
func (tb *TestGoroutineBuilder) Wait() {
	tb.wg.Wait()
}

// StartTestGoroutine is a convenience function for simple test goroutines
// It creates a builder, starts the goroutine, and returns the wait group
// NOTE: The returned WaitGroup should be used for coordination, but the goroutine
// will complete independently. Do not reuse the WaitGroup for other goroutines.
// Note: Add(1) is now called automatically by StartSimple when WithWaitGroup is set
func StartTestGoroutine(name, purpose string, fn func()) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	// Create a fresh builder for each goroutine to avoid state reuse
	builder := NewGoroutine(name, purpose)
	builder.WithWaitGroup(wg).StartSimple(fn)
	return wg
}

// StartTestGoroutines starts multiple test goroutines and returns a combined wait group
func StartTestGoroutines(t *testing.T, count int, namePrefix, purposePrefix string, fn func(id int)) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	for i := 0; i < count; i++ {
		name := namePrefix
		if count > 1 {
			name = fmt.Sprintf("%s_%d", namePrefix, i)
		}
		purpose := purposePrefix
		if count > 1 {
			purpose = fmt.Sprintf("%s (goroutine %d of %d)", purposePrefix, i, count)
		}
		// Note: Add(1) is now called automatically by StartSimple when WithWaitGroup is set
		NewGoroutine(name, purpose).
			WithWaitGroup(wg).
			StartSimple(func() {
				fn(i)
			})
	}
	return wg
}
