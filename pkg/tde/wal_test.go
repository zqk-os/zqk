package tde

import (
	"testing"
	"time"
)

func TestStagingWAL_StageAndLoadActive(t *testing.T) {
	root := t.TempDir()

	wal, err := NewStagingWAL(root)
	if err != nil {
		t.Fatalf("failed to create staging WAL: %v", err)
	}

	env1 := Envelope{
		ID:         "env-1",
		Kind:       "policy",
		TargetID:   "pol-1",
		Operation:  "update",
		PayloadB64: "base64data",
		ExecuteAt:  time.Now().Add(1 * time.Hour),
	}

	if err := wal.Stage(env1); err != nil {
		t.Fatalf("failed to stage env1: %v", err)
	}

	env2 := Envelope{
		ID:        "env-2",
		Kind:      "policy",
		TargetID:  "pol-2",
		Operation: "delete",
		ExecuteAt: time.Now().Add(2 * time.Hour),
	}

	if err := wal.Stage(env2); err != nil {
		t.Fatalf("failed to stage env2: %v", err)
	}

	if err := wal.Sync(); err != nil {
		t.Fatalf("failed to sync: %v", err)
	}
	_ = wal.Close()

	active, err := LoadActive(root)
	if err != nil {
		t.Fatalf("failed to load active: %v", err)
	}

	if len(active) != 2 {
		t.Fatalf("expected 2 active envelopes, got %d", len(active))
	}

	if active["env-1"].TargetID != "pol-1" {
		t.Errorf("expected env1 target pol-1, got %s", active["env-1"].TargetID)
	}
}

func TestStagingWAL_RevokeAndCommit(t *testing.T) {
	root := t.TempDir()

	wal, err := NewStagingWAL(root)
	if err != nil {
		t.Fatalf("failed to create staging WAL: %v", err)
	}

	env1 := Envelope{ID: "env-1", ExecuteAt: time.Now().Add(1 * time.Hour)}
	env2 := Envelope{ID: "env-2", ExecuteAt: time.Now().Add(1 * time.Hour)}
	env3 := Envelope{ID: "env-3", ExecuteAt: time.Now().Add(1 * time.Hour)}

	_ = wal.Stage(env1)
	_ = wal.Stage(env2)
	_ = wal.Stage(env3)

	// Revoke env-2
	_ = wal.Revoke("env-2")

	// Commit env-3
	_ = wal.MarkCommitted("env-3")

	_ = wal.Sync()
	_ = wal.Close()

	active, err := LoadActive(root)
	if err != nil {
		t.Fatalf("failed to load active: %v", err)
	}

	if len(active) != 1 {
		t.Fatalf("expected 1 active envelope, got %d", len(active))
	}

	if _, ok := active["env-1"]; !ok {
		t.Errorf("expected env-1 to be active")
	}
}
