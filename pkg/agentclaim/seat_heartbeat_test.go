package agentclaim

import (
	"path/filepath"
	"testing"
	"time"
)

func TestArmRenewClearSeatHeartbeat(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	seat := "cursor-composer"

	if err := ArmSeatHeartbeat(root, seat, time.Minute); err != nil {
		t.Fatalf("ArmSeatHeartbeat: %v", err)
	}
	path := SeatHeartbeatPath(root, seat)
	if filepath.Base(path) != "seat-cursor-composer.seat.json" {
		t.Fatalf("path base = %q", filepath.Base(path))
	}
	timer, err := LoadSeatHeartbeat(root, seat)
	if err != nil || timer == nil {
		t.Fatalf("LoadSeatHeartbeat: timer=%v err=%v", timer, err)
	}
	if timer.Type != TimerTypeSeatHeartbeat {
		t.Fatalf("type = %q, want %q", timer.Type, TimerTypeSeatHeartbeat)
	}
	if timer.Kind != KindAgentSeat {
		t.Fatalf("kind = %q", timer.Kind)
	}
	armedExpiry := timer.ExpiresAt

	time.Sleep(5 * time.Millisecond)
	if err := RenewSeatHeartbeat(root, seat); err != nil {
		t.Fatalf("RenewSeatHeartbeat: %v", err)
	}
	renewed, err := LoadSeatHeartbeat(root, seat)
	if err != nil || renewed == nil {
		t.Fatalf("load after renew: %v %v", renewed, err)
	}
	if renewed.Misses != 0 {
		t.Fatalf("misses = %d after renew", renewed.Misses)
	}
	if renewed.LastCheckinAt == "" {
		t.Fatal("expected last_checkin_at after renew")
	}
	if renewed.ExpiresAt == armedExpiry && renewed.LastCheckinAt == "" {
		t.Fatal("renew should stamp progress")
	}

	if err := ClearSeatHeartbeat(root, seat); err != nil {
		t.Fatalf("ClearSeatHeartbeat: %v", err)
	}
	gone, err := LoadSeatHeartbeat(root, seat)
	if err != nil {
		t.Fatalf("load after clear: %v", err)
	}
	if gone != nil {
		t.Fatalf("expected nil after clear, got %+v", gone)
	}
}

func TestRenewSeatHeartbeat_absentIsNoop(t *testing.T) {
	t.Parallel()
	if err := RenewSeatHeartbeat(t.TempDir(), "never-armed"); err != nil {
		t.Fatalf("absent renew must be nil, got %v", err)
	}
	if err := RenewSeatHeartbeat(t.TempDir(), "unknown"); err != nil {
		t.Fatalf("unknown agent renew must be nil, got %v", err)
	}
}

func TestArmSeatHeartbeat_validation(t *testing.T) {
	t.Parallel()
	if err := ArmSeatHeartbeat("", "seat", 0); err == nil {
		t.Fatal("expected error for empty root")
	}
	if err := ArmSeatHeartbeat(t.TempDir(), "", 0); err == nil {
		t.Fatal("expected error for empty seat")
	}
}

func TestRearmSeatHeartbeat_backoff(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := ArmSeatHeartbeat(root, "peer-1", time.Minute); err != nil {
		t.Fatalf("arm: %v", err)
	}
	timer, err := LoadSeatHeartbeat(root, "peer-1")
	if err != nil || timer == nil {
		t.Fatalf("load: %v", err)
	}
	timer.Misses = 2
	before := timer.ExpiresAt
	if err := RearmSeatHeartbeat(root, timer, 4*time.Hour); err != nil {
		t.Fatalf("rearm: %v", err)
	}
	after, err := LoadSeatHeartbeat(root, "peer-1")
	if err != nil || after == nil {
		t.Fatalf("load after rearm: %v", err)
	}
	if after.ExpiresAt == before {
		t.Fatal("rearm should extend expires_at")
	}
}

func TestIsSeatHeartbeatPath(t *testing.T) {
	t.Parallel()
	if !IsSeatHeartbeatPath("seat-cursor-composer.seat.json") {
		t.Fatal("expected seat path true")
	}
	if IsSeatHeartbeatPath("ATK-1.checkin.json") {
		t.Fatal("checkin must not match")
	}
	if IsSeatHeartbeatPath("ATK-1.json") {
		t.Fatal("deadline must not match")
	}
}

func TestTimerTypeSeatHeartbeat_distinctFromKillSentinel(t *testing.T) {
	t.Parallel()
	if TimerTypeSeatHeartbeat == "" || TimerTypeSeatHeartbeat == TimerTypeCheckin {
		t.Fatalf("TimerTypeSeatHeartbeat = %q; must be distinct non-empty", TimerTypeSeatHeartbeat)
	}
}
