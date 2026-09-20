package scheduler

import (
	"errors"
	"testing"
	"time"
)

func TestParseDurationOrHoursSuffix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  time.Duration
		ok    bool
	}{
		{name: "empty", input: "", want: 0, ok: false},
		{name: "standard", input: "72h", want: 72 * time.Hour, ok: true},
		{name: "legacy bare hours", input: "48", want: 48 * time.Hour, ok: true},
		{name: "invalid", input: "not-a-duration", want: 0, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseDurationOrHoursSuffix(tt.input)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("parseDurationOrHoursSuffix(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCatchUpAgeLookback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		window, effective time.Duration
		want              time.Duration
	}{
		{name: "default window only", window: time.Hour, effective: 0, want: time.Hour},
		{name: "effective shorter than window", window: time.Hour, effective: 10 * time.Minute, want: 10 * time.Minute},
		{name: "window shorter than effective", window: time.Hour, effective: 2 * time.Hour, want: time.Hour},
		{name: "equal", window: 30 * time.Minute, effective: 30 * time.Minute, want: 30 * time.Minute},
		{name: "long window critical effective", window: 7 * 24 * time.Hour, effective: 10 * time.Minute, want: 10 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := catchUpAgeLookback(tt.window, tt.effective); got != tt.want {
				t.Fatalf("catchUpAgeLookback(%v, %v) = %v, want %v", tt.window, tt.effective, got, tt.want)
			}
		})
	}
}

func TestMetricLimitAuditEventLookback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                           string
		window, deleteAfter, effective time.Duration
		want                           time.Duration
	}{
		{
			name:   "deleteAfter zero falls back to catch-up only",
			window: time.Hour, deleteAfter: 0, effective: 15 * time.Minute,
			want: time.Minute * 15,
		},
		{
			name:   "half retention when no shortening",
			window: time.Hour, deleteAfter: 2 * time.Hour, effective: 2 * time.Hour,
			want: time.Hour,
		},
		{
			name:   "effective shorter than half",
			window: time.Hour, deleteAfter: 2 * time.Hour, effective: 10 * time.Minute,
			want: 10 * time.Minute,
		},
		{
			name:   "window shorter than half retention when not count-shortened",
			window: 45 * time.Minute, deleteAfter: 2 * time.Hour, effective: 0,
			want: 45 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := metricLimitAuditEventLookback(tt.window, tt.deleteAfter, tt.effective)
			if got != tt.want {
				t.Fatalf("metricLimitAuditEventLookback(%v, %v, %v) = %v, want %v",
					tt.window, tt.deleteAfter, tt.effective, got, tt.want)
			}
		})
	}
}

func TestAuditAggregationDegradedSuccessOutcomePatch(t *testing.T) {
	t.Parallel()
	if p := auditAggregationDegradedSuccessOutcomePatch("", nil); p != nil {
		t.Fatalf("empty note: got %#v, want nil", p)
	}
	errBoom := errors.New("hash mismatch on event xyz")
	p := auditAggregationDegradedSuccessOutcomePatch(OutcomeDegradedNoteHashMismatchPartial, errBoom)
	if p == nil {
		t.Fatal("want non-nil patch")
	}
	if p[OutcomeKeyAggregationDegraded] != true {
		t.Fatalf("aggregation_degraded = %v", p[OutcomeKeyAggregationDegraded])
	}
	if p[OutcomeKeyAggregationDegradedNote] != OutcomeDegradedNoteHashMismatchPartial {
		t.Fatalf("note = %v", p[OutcomeKeyAggregationDegradedNote])
	}
	if p[OutcomeKeyAggregationDegradedError] != errBoom.Error() {
		t.Fatalf("error = %v", p[OutcomeKeyAggregationDegradedError])
	}
	p2 := auditAggregationDegradedSuccessOutcomePatch(OutcomeDegradedNoteHashMismatchPartial, nil)
	if _, ok := p2[OutcomeKeyAggregationDegradedError]; ok {
		t.Fatalf("expected no aggregation_degraded_error when err nil: %#v", p2)
	}
}

func TestAuditAggregationRetentionOutcomeFields(t *testing.T) {
	t.Parallel()
	errSample := errors.New("count failed")
	m := auditAggregationRetentionOutcomeFields(time.Hour, 2*time.Hour, 10*time.Minute, 150000, nil)
	if m[OutcomeKeyAggregationWindow] != "1h0m0s" {
		t.Fatalf("aggregation_window = %v", m[OutcomeKeyAggregationWindow])
	}
	if m[OutcomeKeyConfiguredRetention] != "2h0m0s" {
		t.Fatalf("configured_retention = %v", m[OutcomeKeyConfiguredRetention])
	}
	if m[OutcomeKeyEffectiveRetention] != "10m0s" {
		t.Fatalf("effective_retention = %v", m[OutcomeKeyEffectiveRetention])
	}
	if m[OutcomeKeyCatchUpAgeLookback] != "10m0s" {
		t.Fatalf("catch_up_age_lookback = %v", m[OutcomeKeyCatchUpAgeLookback])
	}
	if m[OutcomeKeyMetricLimitAuditAgeLookback] != "10m0s" {
		t.Fatalf("metric_limit_audit_age_lookback = %v", m[OutcomeKeyMetricLimitAuditAgeLookback])
	}
	if m[OutcomeKeyAuditEventCountAtStart] != 150000 {
		t.Fatalf("audit_event_count_at_start = %v", m[OutcomeKeyAuditEventCountAtStart])
	}

	m2 := auditAggregationRetentionOutcomeFields(time.Hour, 2*time.Hour, 2*time.Hour, 0, errSample)
	if _, ok := m2[OutcomeKeyAuditEventCountAtStart]; ok {
		t.Fatalf("expected no audit_event_count_at_start when count err: %#v", m2)
	}
}

func TestRetentionDaysFromJobEnv(t *testing.T) {
	t.Parallel()
	const def = 30

	tests := []struct {
		name string
		env  map[string]string
		want int
	}{
		{name: "nil env", env: nil, want: def},
		{name: "empty map", env: map[string]string{}, want: def},
		{name: "retention days", env: map[string]string{EnvKeyRetentionDays: "7"}, want: 7},
		{name: "retention duration hours", env: map[string]string{EnvKeyRetentionDuration: "72h"}, want: 3},
		{name: "retention duration with h suffix fallback", env: map[string]string{EnvKeyRetentionDuration: "48h"}, want: 2},
		{name: "invalid days does not fall through to duration", env: map[string]string{
			EnvKeyRetentionDays: "nope", EnvKeyRetentionDuration: "240h",
		}, want: def},
		{name: "empty days key uses duration", env: map[string]string{
			EnvKeyRetentionDays: "", EnvKeyRetentionDuration: "24h",
		}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := retentionDaysFromJobEnv(tt.env, def); got != tt.want {
				t.Fatalf("retentionDaysFromJobEnv(...) = %d, want %d", got, tt.want)
			}
		})
	}
}
