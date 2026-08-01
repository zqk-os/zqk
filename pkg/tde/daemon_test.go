package tde

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockMutator struct {
	applied map[string]bool
	err     error
}

func (m *mockMutator) ApplyEnvelope(ctx context.Context, env Envelope) error {
	if m.err != nil {
		return m.err
	}
	m.applied[env.ID] = true
	return nil
}

func TestDaemon_ProcessExpired(t *testing.T) {
	root := t.TempDir()

	wal, err := NewStagingWAL(root)
	if err != nil {
		t.Fatalf("failed to create staging WAL: %v", err)
	}

	mutator := &mockMutator{applied: make(map[string]bool)}
	daemon := NewDaemon(root, wal, mutator, 1*time.Minute)

	// Stage envelopes
	now := time.Now().UTC()
	envExpired := Envelope{ID: "env-expired", ExecuteAt: now.Add(-1 * time.Hour)}
	envFuture := Envelope{ID: "env-future", ExecuteAt: now.Add(1 * time.Hour)}

	_ = wal.Stage(envExpired)
	_ = wal.Stage(envFuture)
	_ = wal.Sync()

	// Run process expired
	if err := daemon.ProcessExpired(context.Background()); err != nil {
		t.Fatalf("ProcessExpired failed: %v", err)
	}

	// Verify only expired was applied
	if !mutator.applied["env-expired"] {
		t.Errorf("expected env-expired to be applied")
	}
	if mutator.applied["env-future"] {
		t.Errorf("expected env-future NOT to be applied")
	}

	// Verify WAL state
	active, err := LoadActive(root)
	if err != nil {
		t.Fatalf("failed to load active: %v", err)
	}

	if len(active) != 1 {
		t.Fatalf("expected 1 active envelope, got %d", len(active))
	}

	if _, ok := active["env-future"]; !ok {
		t.Errorf("expected env-future to be active")
	}
}

func TestDaemon_ProcessExpired_MutatorError(t *testing.T) {
	root := t.TempDir()

	wal, err := NewStagingWAL(root)
	if err != nil {
		t.Fatalf("failed to create staging WAL: %v", err)
	}

	mutator := &mockMutator{applied: make(map[string]bool), err: errors.New("mutator failed")}
	daemon := NewDaemon(root, wal, mutator, 1*time.Minute)

	now := time.Now().UTC()
	envExpired := Envelope{ID: "env-expired", ExecuteAt: now.Add(-1 * time.Hour)}
	_ = wal.Stage(envExpired)
	_ = wal.Sync()

	if err := daemon.ProcessExpired(context.Background()); err != nil {
		t.Fatalf("ProcessExpired failed: %v", err)
	}

	// Verify WAL state
	active, err := LoadActive(root)
	if err != nil {
		t.Fatalf("failed to load active: %v", err)
	}

	// Should still be active because mutator failed
	if len(active) != 1 {
		t.Fatalf("expected 1 active envelope, got %d", len(active))
	}
}

func TestDaemon_ProcessExpired_HardBlock(t *testing.T) {
	root := t.TempDir()

	wal, err := NewStagingWAL(root)
	if err != nil {
		t.Fatalf("failed to create staging WAL: %v", err)
	}

	mutator := &mockMutator{applied: make(map[string]bool)}
	daemon := NewDaemon(root, wal, mutator, 1*time.Minute)

	now := time.Now().UTC()
	// High-risk kind: policy
	envHighRisk := Envelope{ID: "env-high-risk", Kind: "policy", ExecuteAt: now.Add(-1 * time.Hour)}
	// Normal kind: task
	envNormal := Envelope{ID: "env-normal", Kind: "task", ExecuteAt: now.Add(-1 * time.Hour)}

	_ = wal.Stage(envHighRisk)
	_ = wal.Stage(envNormal)
	_ = wal.Sync()

	if err := daemon.ProcessExpired(context.Background()); err != nil {
		t.Fatalf("ProcessExpired failed: %v", err)
	}

	// Verify hard-block
	if mutator.applied["env-high-risk"] {
		t.Errorf("expected env-high-risk to be BLOCKED (not applied)")
	}
	if !mutator.applied["env-normal"] {
		t.Errorf("expected env-normal to be applied")
	}

	// Verify WAL state
	active, err := LoadActive(root)
	if err != nil {
		t.Fatalf("failed to load active: %v", err)
	}

	// High-risk should still be active (blocked), normal should be gone (committed)
	if _, ok := active["env-high-risk"]; !ok {
		t.Errorf("expected env-high-risk to remain active (blocked)")
	}
}
