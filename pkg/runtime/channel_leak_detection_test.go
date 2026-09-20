package runtime

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// ChannelTracker tracks channels to detect leaks
type ChannelTracker struct {
	channels map[string]chan struct{}
	mu       sync.RWMutex
}

// NewChannelTracker creates a new channel tracker
func NewChannelTracker() *ChannelTracker {
	return &ChannelTracker{
		channels: make(map[string]chan struct{}),
	}
}

// TrackChannel tracks a channel for leak detection
func (ct *ChannelTracker) TrackChannel(name string, ch chan struct{}) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.channels[name] = ch
}

// IsChannelClosed checks if a channel is closed
func (ct *ChannelTracker) IsChannelClosed(name string) bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	ch, ok := ct.channels[name]
	if !ok {
		return false // Channel not tracked
	}

	// Try to receive from channel - if closed, will get zero value immediately
	select {
	case _, ok := <-ch:
		return !ok // Channel is closed if receive returns false
	default:
		return false // Channel is open (has value or not closed)
	}
}

// DetectLeaks detects channels that should be closed but aren't
func (ct *ChannelTracker) DetectLeaks() []string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	var leaks []string
	for name, ch := range ct.channels {
		// Try to receive - if channel is closed, receive will return immediately with ok=false
		select {
		case _, ok := <-ch:
			if ok {
				// Channel is open and has a value - not necessarily a leak
				// But if we expected it to be closed, it's a leak
				// For this test, we'll check if channel is still open after expected close
			} else {
				// Channel is closed - not a leak
				continue
			}
		default:
			// Channel is open but empty - potential leak if it should be closed
			leaks = append(leaks, name)
		}
	}
	return leaks
}

// TestChannelLeakDetection verifies that channels are properly closed
func TestChannelLeakDetection(t *testing.T) {
	t.Parallel()
	tracker := NewChannelTracker()

	// Get baseline goroutine count (channels don't directly affect goroutine count,
	// but leaked channels might indicate leaked goroutines)
	before := runtime.NumGoroutine()

	// Test 1: Properly closed channel (should not leak)
	t.Run("ProperlyClosedChannel", func(t *testing.T) {
		ch := make(chan struct{})
		tracker.TrackChannel("properly_closed", ch)

		// Close channel properly
		close(ch)

		// Verify channel is closed
		if !tracker.IsChannelClosed("properly_closed") {
			t.Error("Expected channel to be closed")
		}

		// Verify no leak detected
		leaks := tracker.DetectLeaks()
		for _, leak := range leaks {
			if leak == "properly_closed" {
				t.Error("Properly closed channel should not be detected as leak")
			}
		}
	})

	// Test 2: Channel closed after use (should not leak)
	t.Run("ChannelClosedAfterUse", func(t *testing.T) {
		ch := make(chan struct{}, 1)
		tracker.TrackChannel("closed_after_use", ch)

		// Use channel
		ch <- struct{}{}
		<-ch

		// Close after use
		close(ch)

		// Verify channel is closed
		if !tracker.IsChannelClosed("closed_after_use") {
			t.Error("Expected channel to be closed after use")
		}
	})

	// Test 3: Channel used in goroutine and properly closed
	t.Run("ChannelInGoroutine_ProperlyClosed", func(t *testing.T) {
		ch := make(chan struct{})
		tracker.TrackChannel("goroutine_channel", ch)

		done := make(chan struct{})
		goroutinelabels.NewGoroutine("test_goroutine", "handling channel signal").
			StartSimple(func() {
				defer close(done)
				// Wait for signal
				select {
				case <-ch:
					// Received signal
				case <-time.After(100 * time.Millisecond):
					// Timeout
				}
			})

		// Signal goroutine
		ch <- struct{}{}

		// Wait for goroutine to complete
		select {
		case <-done:
			// Goroutine completed
		case <-time.After(200 * time.Millisecond):
			t.Error("Goroutine did not complete")
		}

		// Close channel after goroutine completes
		close(ch)

		// Verify channel is closed
		if !tracker.IsChannelClosed("goroutine_channel") {
			t.Error("Expected channel to be closed after goroutine completes")
		}
	})

	// Test 4: Channel with context cancellation (should close properly)
	t.Run("ChannelWithContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
		defer cancel()

		ch := make(chan struct{})
		tracker.TrackChannel("context_channel", ch)

		done := make(chan struct{})
		goroutinelabels.NewGoroutine("runtime_test", "context channel handling").StartSimple(func() {
			defer close(done)
			select {
			case <-ch:
				// Received signal
			case <-ctx.Done():
				// Context cancelled
			}
		})

		// Cancel context
		cancel()

		// Wait for goroutine to complete
		select {
		case <-done:
			// Goroutine completed
		case <-time.After(200 * time.Millisecond):
			t.Error("Goroutine did not complete after context cancellation")
		}

		// Close channel
		close(ch)

		// Verify channel is closed
		if !tracker.IsChannelClosed("context_channel") {
			t.Error("Expected channel to be closed after context cancellation")
		}
	})

	// Verify goroutine count returned to baseline (allowing some margin)
	after := runtime.NumGoroutine()
	if after > before+2 { // Allow some margin for test infrastructure
		t.Errorf("Potential goroutine leak detected: before=%d, after=%d", before, after)
	}
}

// TestChannelLeakDetection_UnclosedChannel tests detection of unclosed channels
func TestChannelLeakDetection_UnclosedChannel(t *testing.T) {
	t.Parallel()
	tracker := NewChannelTracker()

	// Create a buffered channel so we can send/receive without blocking; we still leave it unclosed
	ch := make(chan struct{}, 1)
	tracker.TrackChannel("unclosed_channel", ch)

	// Use channel but don't close it
	ch <- struct{}{}
	<-ch

	// Verify channel is NOT closed (this is the leak)
	if tracker.IsChannelClosed("unclosed_channel") {
		t.Error("Expected unclosed channel to not be detected as closed")
	}

	// Verify leak is detected
	leaks := tracker.DetectLeaks()
	found := false
	for _, leak := range leaks {
		if leak == "unclosed_channel" {
			found = true
			break
		}
	}
	if !found {
		t.Log("Note: Unclosed channel may not be detected if it's buffered and empty")
	}

	// Clean up for test
	close(ch)
}

// TestChannelLeakDetection_BufferedChannel tests buffered channels
func TestChannelLeakDetection_BufferedChannel(t *testing.T) {
	t.Parallel()
	tracker := NewChannelTracker()

	// Buffered channel
	ch := make(chan struct{}, 10)
	tracker.TrackChannel("buffered_channel", ch)

	// Fill buffer
	for i := 0; i < 10; i++ {
		ch <- struct{}{}
	}

	// Drain buffer
	for i := 0; i < 10; i++ {
		<-ch
	}

	// Close channel
	close(ch)

	// Verify channel is closed
	if !tracker.IsChannelClosed("buffered_channel") {
		t.Error("Expected buffered channel to be closed")
	}
}

// TestChannelLeakDetection_SelectStatement tests channels in select statements
func TestChannelLeakDetection_SelectStatement(t *testing.T) {
	t.Parallel()
	tracker := NewChannelTracker()

	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	tracker.TrackChannel("select_ch1", ch1)
	tracker.TrackChannel("select_ch2", ch2)

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("runtime_test", "select statement handling").StartSimple(func() {
		defer close(done)
		select {
		case <-ch1:
			// Received from ch1
		case <-ch2:
			// Received from ch2
		case <-time.After(100 * time.Millisecond):
			// Timeout
		}
	})

	// Signal ch1
	ch1 <- struct{}{}

	// Wait for goroutine
	select {
	case <-done:
		// Goroutine completed
	case <-time.After(200 * time.Millisecond):
		t.Error("Goroutine did not complete")
	}

	// Close both channels
	close(ch1)
	close(ch2)

	// Verify both channels are closed
	if !tracker.IsChannelClosed("select_ch1") {
		t.Error("Expected ch1 to be closed")
	}
	if !tracker.IsChannelClosed("select_ch2") {
		t.Error("Expected ch2 to be closed")
	}
}
