package vitality

import (
	"context"
	"testing"
)

func TestVitalityMonitor(t *testing.T) {
	// Test with a process that is running (e.g., current process or go/test/sh/zsh)
	m := NewVitalityMonitor("go")
	if m.processName != "go" {
		t.Fatalf("expected processName 'go', got %q", m.processName)
	}

	ctx := context.Background()
	_ = m.EnsureRunning(ctx)

	// Test with a process name that definitely does not exist
	mNonExistent := NewVitalityMonitor("non_existent_process_xyz_123456789")
	err := mNonExistent.EnsureRunning(ctx)
	if err != nil {
		t.Fatalf("unexpected error from EnsureRunning: %v", err)
	}

	if err := mNonExistent.restart(); err != nil {
		t.Fatalf("unexpected error from restart: %v", err)
	}
}
