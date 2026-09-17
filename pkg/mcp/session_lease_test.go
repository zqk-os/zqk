package mcp

import (
	"testing"
	"time"
)

func TestSessionLeaseExpired(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	ttl := 30 * time.Minute

	tests := []struct {
		name         string
		lastActivity string
		ttl          time.Duration
		want         bool
	}{
		{name: "fresh", lastActivity: now.Add(-5 * time.Minute).Format(time.RFC3339), ttl: ttl, want: false},
		{name: "exactly_at_ttl_not_expired", lastActivity: now.Add(-ttl).Format(time.RFC3339), ttl: ttl, want: false},
		{name: "past_ttl", lastActivity: now.Add(-ttl - time.Second).Format(time.RFC3339), ttl: ttl, want: true},
		{name: "empty_activity", lastActivity: "", ttl: ttl, want: true},
		{name: "bad_timestamp", lastActivity: "not-a-time", ttl: ttl, want: true},
		{name: "ttl_disabled", lastActivity: "", ttl: 0, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := SessionLeaseExpired(tc.lastActivity, tc.ttl, now); got != tc.want {
				t.Fatalf("SessionLeaseExpired(%q, %v) = %v, want %v", tc.lastActivity, tc.ttl, got, tc.want)
			}
		})
	}
}

func TestTouchSessionLastActivity_noSessionID(t *testing.T) {
	t.Parallel()
	s := NewServer()
	TouchSessionLastActivity(s)
	if !s.GetSessionLastActivityAt().IsZero() {
		t.Fatal("expected no in-memory activity when session ID unset")
	}
}

func TestTouchSessionLastActivity_throttlesPersist(t *testing.T) {
	t.Parallel()
	s := NewServer()
	s.SetCurrentSessionID("MCP-999")

	TouchSessionLastActivity(s)
	first := s.GetSessionLastActivityAt()
	if first.IsZero() {
		t.Fatal("expected first touch to set in-memory activity")
	}
	s.sessionActivityMu.Lock()
	firstPersist := s.sessionLastPersistAt
	s.sessionActivityMu.Unlock()
	if firstPersist.IsZero() {
		t.Fatal("expected first touch to schedule persist (set sessionLastPersistAt)")
	}

	// Second touch within persist interval still updates in-memory activity.
	time.Sleep(2 * time.Millisecond)
	TouchSessionLastActivity(s)
	second := s.GetSessionLastActivityAt()
	if second.Before(first) {
		t.Fatalf("activity moved backwards: first=%v second=%v", first, second)
	}

	s.sessionActivityMu.Lock()
	secondPersist := s.sessionLastPersistAt
	s.sessionActivityMu.Unlock()
	if !secondPersist.Equal(firstPersist) {
		t.Fatalf("persist throttle broken: firstPersist=%v secondPersist=%v", firstPersist, secondPersist)
	}
}

func TestCurrentSessionLeaseExpired(t *testing.T) {
	t.Parallel()
	s := NewServer()
	if s.CurrentSessionLeaseExpired(DefaultSessionLeaseTTL) {
		t.Fatal("no session should not report expired lease")
	}
	s.SetCurrentSessionID("MCP-001")
	if !s.CurrentSessionLeaseExpired(DefaultSessionLeaseTTL) {
		t.Fatal("session with never-touched activity should be expired")
	}
	TouchSessionLastActivity(s)
	if s.CurrentSessionLeaseExpired(DefaultSessionLeaseTTL) {
		t.Fatal("fresh touch should not be expired under default TTL")
	}
	if !s.CurrentSessionLeaseExpired(time.Nanosecond) {
		t.Fatal("nanosecond TTL should expire immediately after touch")
	}
}
