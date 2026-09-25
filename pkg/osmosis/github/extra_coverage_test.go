// BLI-STARTER-COMMUNITY-039 / PRI-STARTER-COMMUNITY-039 coverage elevation
package github

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/metrics"
)

type stubLimiter struct{ allow bool }

func (s stubLimiter) Allow(string) bool { return s.allow }

type stubTracker struct {
	limited int
	execs   int
}

func (s *stubTracker) RecordExecution(context.Context, string, []string, time.Duration, int, error) {
	s.execs++
}
func (s *stubTracker) RecordRateLimitExceeded(context.Context, string)             { s.limited++ }
func (s *stubTracker) RecordCacheHit(context.Context, string)                      {}
func (s *stubTracker) RecordCacheMiss(context.Context, string)                     {}
func (s *stubTracker) GetCacheHitRatios() map[string]float64                       { return nil }
func (s *stubTracker) RecordIPCLatency(context.Context, string, time.Duration)     {}
func (s *stubTracker) RecordAgentTokenUsage(context.Context, string, int64, int64) {}
func (s *stubTracker) RecordGhostDriftMTTR(context.Context, string, time.Duration) {}
func (s *stubTracker) RecordPersonaSkillInvocation(context.Context, string, string, time.Duration, bool) {
}
func (s *stubTracker) GetIPCHistograms() map[string]*metrics.PrometheusHistogram        { return nil }
func (s *stubTracker) GetGhostDriftHistograms() map[string]*metrics.PrometheusHistogram { return nil }

func TestExtraInterceptor(t *testing.T) {
	ctx := context.Background()
	tr := &stubTracker{}
	blocked := NewInterceptor("true", stubLimiter{allow: false}, tr)
	if _, err := blocked.Execute(ctx, []string{"ok"}); err != ErrRateLimitExceeded || tr.limited != 1 {
		t.Fatalf("rate limit = %v limited=%d", err, tr.limited)
	}
	if _, err := NewInterceptor("true", stubLimiter{allow: true}, nil).Execute(ctx, []string{"block-me"}); err != ErrExecutionBlocked {
		t.Fatal(err)
	}
	out, err := NewInterceptor("true", stubLimiter{allow: true}, tr).Execute(ctx, []string{})
	if err != nil {
		t.Fatal(err)
	}
	_ = out
	if _, err := NewInterceptor("false", stubLimiter{allow: true}, tr).Execute(ctx, []string{}); err == nil {
		t.Fatal("expected false binary failure")
	}
	if NewInterceptor("git", nil, nil) == nil {
		t.Fatal("constructor")
	}
}
