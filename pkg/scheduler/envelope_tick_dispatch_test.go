package scheduler

import (
	"context"
	"testing"
)

func TestDispatchEnvelopeTickResolvedJobs_Matrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		env                  map[string]string
		resolvedJobTypes     []string
		expectedMode         string
		expectedExpand       bool
		expectedSkipNotAllow int
		expectedShadowCount  int
		expectedAllowExtra   int
		expectedDenyExtra    int
		expectedRateLimited  int
	}{
		{
			name:                 "Mode Off (default) evaluates to off with no triggers",
			env:                  map[string]string{},
			resolvedJobTypes:     []string{"lifecycle_check"},
			expectedMode:         "off",
			expectedExpand:       false,
			expectedSkipNotAllow: 0,
			expectedShadowCount:  0,
		},
		{
			name: "Shadow Mode with Core Allowlist",
			env: map[string]string{
				"ENVELOPE_TICK_DISPATCH_MODE": "shadow",
			},
			resolvedJobTypes:    []string{"lifecycle_check", "cap_orchestrator"},
			expectedMode:        "shadow",
			expectedExpand:      false,
			expectedShadowCount: 2,
		},
		{
			name: "Shadow Mode rejects Expanded type when EXPAND is off",
			env: map[string]string{
				"ENVELOPE_TICK_DISPATCH_MODE": "shadow",
			},
			resolvedJobTypes:     []string{"object_validation"},
			expectedMode:         "shadow",
			expectedExpand:       false,
			expectedSkipNotAllow: 1,
			expectedShadowCount:  0,
		},
		{
			name: "Shadow Mode accepts Expanded type when EXPAND is on",
			env: map[string]string{
				"ENVELOPE_TICK_DISPATCH_MODE":   "shadow",
				"ENVELOPE_TICK_DISPATCH_EXPAND": "true",
			},
			resolvedJobTypes:    []string{"object_validation"},
			expectedMode:        "shadow",
			expectedExpand:      true,
			expectedShadowCount: 1,
		},
		{
			name: "Allowlist Extra adds custom job type",
			env: map[string]string{
				"ENVELOPE_TICK_DISPATCH_MODE":            "shadow",
				"ENVELOPE_TICK_DISPATCH_ALLOWLIST_EXTRA": "custom_audit",
			},
			resolvedJobTypes:    []string{"custom_audit"},
			expectedMode:        "shadow",
			expectedShadowCount: 1,
			expectedAllowExtra:  1,
		},
		{
			name: "Deny Extra overrides allowlist",
			env: map[string]string{
				"ENVELOPE_TICK_DISPATCH_MODE":       "shadow",
				"ENVELOPE_TICK_DISPATCH_DENY_EXTRA": "lifecycle_check",
			},
			resolvedJobTypes:     []string{"lifecycle_check"},
			expectedMode:         "shadow",
			expectedSkipNotAllow: 0,
			expectedShadowCount:  0,
			expectedDenyExtra:    1,
		},
		{
			name: "Max Triggers rate-limits dispatch count",
			env: map[string]string{
				"ENVELOPE_TICK_DISPATCH_MODE":         "shadow",
				"ENVELOPE_TICK_DISPATCH_MAX_TRIGGERS": "1",
			},
			resolvedJobTypes:    []string{"lifecycle_check", "cap_orchestrator"},
			expectedMode:        "shadow",
			expectedShadowCount: 1,
			expectedRateLimited: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &Scheduler{
				TestHookEnvelopeTickTargetForJobType: func(jt string) string {
					return "SCH-test-target-" + jt
				},
			}
			job := &ScheduledJob{
				EnvironmentVariables: tc.env,
			}

			outcome := s.DispatchEnvelopeTickResolvedJobs(context.Background(), job, tc.resolvedJobTypes)

			if outcome.Mode != tc.expectedMode {
				t.Errorf("expected mode %q, got %q", tc.expectedMode, outcome.Mode)
			}
			if outcome.Expand != tc.expectedExpand {
				t.Errorf("expected expand %v, got %v", tc.expectedExpand, outcome.Expand)
			}
			if outcome.SkipNotAllowlisted != tc.expectedSkipNotAllow {
				t.Errorf("expected SkipNotAllowlisted %d, got %d", tc.expectedSkipNotAllow, outcome.SkipNotAllowlisted)
			}
			if outcome.ShadowWouldTrigger != tc.expectedShadowCount {
				t.Errorf("expected ShadowWouldTrigger %d, got %d", tc.expectedShadowCount, outcome.ShadowWouldTrigger)
			}
			if tc.expectedAllowExtra > 0 && outcome.AllowlistExtraN != tc.expectedAllowExtra {
				t.Errorf("expected AllowlistExtraN %d, got %d", tc.expectedAllowExtra, outcome.AllowlistExtraN)
			}
			if tc.expectedDenyExtra > 0 && outcome.DenyExtraN != tc.expectedDenyExtra {
				t.Errorf("expected DenyExtraN %d, got %d", tc.expectedDenyExtra, outcome.DenyExtraN)
			}
			if tc.expectedRateLimited > 0 && outcome.SkipRateLimited != tc.expectedRateLimited {
				t.Errorf("expected SkipRateLimited %d, got %d", tc.expectedRateLimited, outcome.SkipRateLimited)
			}
		})
	}
}
