package agentfeed

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
)

// testFakeWakeAdapter records wake requests without invoking shell membranes.
type testFakeWakeAdapter struct {
	mu    sync.Mutex
	calls []PeerWakeRequest
}

func (f *testFakeWakeAdapter) Wake(_ context.Context, req PeerWakeRequest) (PeerWakeAdapterResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	return PeerWakeAdapterResult{Live: true, Transport: "test"}, nil
}

func (f *testFakeWakeAdapter) last() PeerWakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return PeerWakeRequest{}
	}
	return f.calls[len(f.calls)-1]
}

// TestSelfWakeExcluded verifies that waking oneself is skipped/excluded.
func TestSelfWakeExcluded(t *testing.T) {
	fake := &testFakeWakeAdapter{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res := WakePeerOpts(ctx, WakePeerOptions{
		ProjectRoot:  t.TempDir(),
		ToAgentID:    "auth-01",
		FromAgentID:  "auth-01", // same as ToAgentID — self-reference
		Message:      "ping myself",
		DeliveryMode: datacell.DeliveryModeNotify,
		Adapter:      fake,
	})

	// Should skip because author === recipient
	if res.Skipped != skWakeAuthorExcluded {
		t.Fatalf("expected Skipped=%q, got %q; error=%q", skWakeAuthorExcluded, res.Skipped, res.Error)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("zero wake calls expected for self-wake; got %d", len(fake.calls))
	}
	if !res.WakeAuthorExcluded {
		t.Fatal("expected WakeAuthorExcluded=true")
	}
}

// TestSelfWakeSkippedWithSessionBinding verifies that when FromAgentID is empty but ZQK_SESSION
// is set, the session value is used as the effective author and self-wake is still excluded.
func TestSelfWakeSkippedWithSessionBinding(t *testing.T) {
	fake := &testFakeWakeAdapter{}

	orig := os.Getenv(SessionEnvKey)
	defer func() {
		if orig == "" {
			_ = os.Unsetenv(SessionEnvKey)
		} else {
			_ = os.Setenv(SessionEnvKey, orig)
		}
	}()

	// Scenario: ZQK_SESSION is set.  FromAgentID should be populated from it.
	t.Setenv(SessionEnvKey, "session-abc")

	ctx := context.Background()
	res := WakePeerOpts(ctx, WakePeerOptions{
		ProjectRoot:  t.TempDir(),
		ToAgentID:    "session-abc", // matches ZQK_SESSION
		Message:      "ping session",
		DeliveryMode: datacell.DeliveryModeNotify,
		Adapter:      fake,
	})

	if res.Skipped != skWakeAuthorExcluded {
		t.Fatalf("expected Skipped=%q when ZQK_SESSION==ToAgentID; got %q", skWakeAuthorExcluded, res.Skipped)
	}

	// Scenario: different ToAgentID — should NOT be excluded.
	fake = &testFakeWakeAdapter{}
	ctx = context.Background()
	t.Setenv(SessionEnvKey, "session-xyz")
	res = WakePeerOpts(ctx, WakePeerOptions{
		ProjectRoot:  t.TempDir(),
		ToAgentID:    "different-peer", // doesn't match ZQK_SESSION
		Message:      "ping peer",
		DeliveryMode: datacell.DeliveryModeNotify,
		Adapter:      fake,
	})

	if res.Skipped != "" {
		t.Fatalf("expected no skip for different peer; got %q", res.Skipped)
	}
}

// TestSeatWorkerBound verifies the SeatWorkerBound helper.
func TestSeatWorkerBound(t *testing.T) {
	testCases := []struct {
		name     string
		setValue string // empty -> unbind via Unsetenv; non-empty -> Setenv
	}{
		{"unbound_no_env", ""},
		{"bound_with_env", "test-session-value"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setValue == "" {
				os.Unsetenv(SessionEnvKey)
			} else {
				t.Setenv(SessionEnvKey, tc.setValue)
			}
			ok, val := SeatWorkerBound()
			if ok != (tc.setValue != "") {
				t.Fatalf("SeatWorkerBound() = %v, want %v", ok, tc.setValue != "")
			}
			if tc.setValue != "" && val != "test-session-value" {
				t.Fatalf("expected session=%q, got %q", "test-session-value", val)
			}
			if tc.setValue == "" && val != "" {
				t.Fatalf("expected empty session when unbound, got %q", val)
			}
		})
	}
}

// TestNormalWakeUnaffected verifies non-self wake still proceeds normally.
func TestNormalWakeUnaffected(t *testing.T) {
	fake := &testFakeWakeAdapter{}
	ctx := context.Background()

	res := WakePeerOpts(ctx, WakePeerOptions{
		ProjectRoot:  t.TempDir(),
		ToAgentID:    "peer-01",
		FromAgentID:  "sender-02", // different from ToAgentID
		Message:      "ping peer",
		DeliveryMode: datacell.DeliveryModeNotify,
		Adapter:      fake,
	})

	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !res.Attempted {
		t.Fatal("expected Attempted=true for normal peer wake")
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected exactly one call; got %d", len(fake.calls))
	}
	if res.WakeAuthorExcluded {
		t.Fatal("expected WakeAuthorExcluded=false for non-self wake")
	}
}

// TestResolveFromAgentID tests the session binding fallback.
func TestResolveFromAgentID(t *testing.T) {
	tests := []struct {
		name        string
		fromAgentID string // what's set in opts.FromAgentID
		sessionEnv  string // ZQK_SESSION (empty = unset)
		want        string
	}{
		{
			name:        "explicit_wins",
			fromAgentID: "explicit-author",
			sessionEnv:  "session-abc",
			want:        "explicit-author",
		},
		{
			name:       "session_fallback",
			fromAgentID: "",
			sessionEnv:  "bound-session",
			want:        "bound-session",
		},
		{
			name:       "default_when_unbound",
			fromAgentID: "",
			sessionEnv:  "", // unset
			want:        FromAgentID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sessionEnv == "" {
				os.Unsetenv(SessionEnvKey)
			} else {
				t.Setenv(SessionEnvKey, tt.sessionEnv)
			}
			opts := WakePeerOptions{FromAgentID: tt.fromAgentID}
			got := resolveFromAgentID(opts)
			if got != tt.want {
				t.Fatalf("resolveFromAgentID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPeerWakeResult_ExcludedField verifies WakeAuthorExcluded field propagates.
func TestPeerWakeResult_ExcludedField(t *testing.T) {
	res := PeerWakeResult{
		Skipped:            skWakeAuthorExcluded,
		SkippedReason:      "self_wake",
		WakeAuthorExcluded: true,
	}
	if !res.WakeAuthorExcluded {
		t.Fatal("expected WakeAuthorExcluded=true")
	}
	if res.Skipped != "author_excluded" {
		t.Fatalf("Skipped = %q, want %q", res.Skipped, skWakeAuthorExcluded)
	}
}

// TestNoSelfMatchWhenEmptyToAgentID verifies that when ToAgentID is empty,
// no self-wake exclusion can occur and wake proceeds normally.
func TestNoSelfMatchWhenEmptyToAgentID(t *testing.T) {
	fake := &testFakeWakeAdapter{}
	ctx := context.Background()

	res := WakePeerOpts(ctx, WakePeerOptions{
		ProjectRoot:  t.TempDir(),
		ToAgentID:    "",
		FromAgentID:  "any-agent",
		Message:      "ping any",
		DeliveryMode: datacell.DeliveryModeNotify,
		Adapter:      fake,
	})

	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	// Empty ToAgentID means no self-match possible; wake should proceed.
	if !res.Attempted {
		t.Fatal("expected Attempted=true when ToAgentID is empty")
	}
}
