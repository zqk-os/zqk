package ux

import (
	"errors"
	"testing"
	"time"

	"github.com/briandowns/spinner"
)

func TestStepTracker_CompleteAndFail(t *testing.T) {
	origTTY, origSuppressed := isTTY, suppressed
	t.Cleanup(func() {
		SetInteractive(origTTY)
		SetSuppressed(origSuppressed)
	})

	// Test non-interactive mode
	SetInteractive(false)
	SetSuppressed(false)

	tracker := NewStepTracker("Validating graph schema")
	if tracker.Elapsed() < 0 {
		t.Fatalf("expected positive elapsed duration, got %v", tracker.Elapsed())
	}
	tracker.Update("checking node integrity")
	tracker.Complete("Schema validation successful")

	// Verify idempotency on completed tracker
	tracker.Update("should not panic or do work")
	tracker.Complete("already completed")

	// Test failure path
	failTracker := NewStepTracker("Syncing remote CAS")
	failTracker.Fail("Sync failed", errors.New("connection reset by peer"))
	failTracker.Fail("Sync failed again", nil)

	// Test interactive mode with mock spinner
	SetInteractive(true)
	globalSpinner = spinner.New(spinner.CharSets[14], time.Millisecond)

	intTracker := NewStepTracker("Rebuilding index")
	time.Sleep(5 * time.Millisecond)
	intTracker.Update("flushing WAL")
	if intTracker.Elapsed() < 5*time.Millisecond {
		t.Errorf("expected elapsed >= 5ms, got %v", intTracker.Elapsed())
	}
	intTracker.Complete("Index rebuilt successfully")
}

func TestSuppressionControl(t *testing.T) {
	origTTY, origSuppressed := isTTY, suppressed
	t.Cleanup(func() {
		SetInteractive(origTTY)
		SetSuppressed(origSuppressed)
	})

	SetInteractive(true)
	SetSuppressed(false)
	if !IsInteractive() {
		t.Errorf("expected IsInteractive to be true")
	}

	SetSuppressed(true)
	if IsInteractive() {
		t.Errorf("expected IsInteractive to be false when suppressed")
	}
	if !IsSuppressed() {
		t.Errorf("expected IsSuppressed to be true")
	}

	// Starting spinner while suppressed should not crash and should fall back safely
	StartSpinner("suppressed operation")
	StopSpinner(true, "suppressed finished")
}
