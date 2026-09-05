package validation

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestParseEffortHours(t *testing.T) {
	cases := []struct {
		in     string
		want   float64
		wantOK bool
	}{
		{"4h", 4, true},
		{"1d", 24, true},
		{"0.5d", 12, true},
		{"2-3 days", 72, true},
		{"small", 0, false},
		{"", 0, false},
		{"8 hours", 8, true},
	}
	for _, tc := range cases {
		got, ok := ParseEffortHours(tc.in)
		if ok != tc.wantOK {
			t.Fatalf("%q ok=%v want %v", tc.in, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Fatalf("%q got %v want %v", tc.in, got, tc.want)
		}
	}
}

func TestValidateActualEffortWithinWallClock(t *testing.T) {
	base := map[string]any{
		objects.FieldKeyKind:      objects.KindBacklogItem,
		objects.FieldKeyCreatedAt: "2026-08-05T10:00:00Z",
		objects.FieldKeyUpdatedAt: "2026-08-05T12:00:00Z", // 2h wall
	}

	okObj := map[string]any{}
	for k, v := range base {
		okObj[k] = v
	}
	okObj[objects.FieldKeyActualEffort] = "1h"
	if errs := ValidateActualEffortWithinWallClock(okObj); len(errs) != 0 {
		t.Fatalf("1h within 2h wall: %v", errs)
	}

	bad := map[string]any{}
	for k, v := range base {
		bad[k] = v
	}
	bad[objects.FieldKeyActualEffort] = "1d"
	errs := ValidateActualEffortWithinWallClock(bad)
	if len(errs) != 1 {
		t.Fatalf("1d vs 2h wall: want 1 err, got %v", errs)
	}
	if errs[0].Rule != RuleActualEffortWallClock {
		t.Fatalf("rule=%q", errs[0].Rule)
	}

	// completed_at preferred over updated_at
	withCompleted := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyCreatedAt:    "2026-08-05T10:00:00Z",
		objects.FieldKeyUpdatedAt:    "2026-08-05T20:00:00Z",
		objects.FieldKeyCompletedAt:  "2026-08-05T11:00:00Z", // 1h wall
		objects.FieldKeyActualEffort: "2h",
	}
	if errs := ValidateActualEffortWithinWallClock(withCompleted); len(errs) != 1 {
		t.Fatalf("completed_at 1h vs actual 2h: %v", errs)
	}

	qual := map[string]any{
		objects.FieldKeyCreatedAt:    "2026-08-05T10:00:00Z",
		objects.FieldKeyUpdatedAt:    "2026-08-05T10:01:00Z",
		objects.FieldKeyActualEffort: "small",
	}
	if errs := ValidateActualEffortWithinWallClock(qual); len(errs) != 0 {
		t.Fatalf("qualitative skipped: %v", errs)
	}

	policyLeftover := map[string]any{
		objects.FieldKeyKind:         objects.KindPolicy,
		objects.FieldKeyCreatedAt:    "2026-08-05T10:00:00Z",
		objects.FieldKeyUpdatedAt:    "2026-08-05T12:00:00Z",
		objects.FieldKeyActualEffort: "1d",
	}
	if errs := ValidateActualEffortWithinWallClock(policyLeftover); len(errs) != 0 {
		t.Fatalf("non-effort_aware leftover must not diagnose: %v", errs)
	}
}

func TestWallClockActualEffortString(t *testing.T) {
	got, ok := WallClockActualEffortString(map[string]any{
		objects.FieldKeyCreatedAt: "2026-08-20T10:01:08Z",
		objects.FieldKeyUpdatedAt: "2026-08-20T10:37:23Z",
	})
	if !ok || got != "0.60h" {
		t.Fatalf("got %q ok=%v want 0.60h", got, ok)
	}
	if _, ok := WallClockActualEffortString(map[string]any{}); ok {
		t.Fatal("expected no span without timestamps")
	}
	// started_at is the work clock; created_at is only the fallback.
	got, ok = WallClockActualEffortString(map[string]any{
		objects.FieldKeyCreatedAt:   "2026-08-20T08:00:00Z",
		objects.FieldKeyStartedAt:   "2026-08-20T10:00:00Z",
		objects.FieldKeyCompletedAt: "2026-08-20T11:00:00Z",
	})
	if !ok || got != "1h" {
		t.Fatalf("started_at span got %q ok=%v want 1h", got, ok)
	}
}

func TestClampActualEffortToWallClock(t *testing.T) {
	obj := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyCreatedAt:    "2026-08-04T06:40:09Z",
		objects.FieldKeyUpdatedAt:    "2026-08-05T15:37:37Z", // ~32.96h
		objects.FieldKeyActualEffort: "2 days",
	}
	clamped, from, to := ClampActualEffortToWallClock(obj)
	if !clamped {
		t.Fatal("expected clamp")
	}
	if from != "2 days" || to != "32h" {
		t.Fatalf("from=%q to=%q", from, to)
	}
	if obj[objects.FieldKeyActualEffort] != "32h" {
		t.Fatalf("obj actual=%v", obj[objects.FieldKeyActualEffort])
	}
	if errs := ValidateActualEffortWithinWallClock(obj); len(errs) != 0 {
		t.Fatalf("after clamp should validate: %v", errs)
	}

	// Idempotent when already within wall
	clamped2, _, _ := ClampActualEffortToWallClock(obj)
	if clamped2 {
		t.Fatal("second clamp should be no-op")
	}
}

func TestApplyWorkEnvelopeWallClockPolicy(t *testing.T) {
	base := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyCreatedAt:    "2026-08-05T10:00:00Z",
		objects.FieldKeyUpdatedAt:    "2026-08-05T12:00:00Z",
		objects.FieldKeyActualEffort: "1d",
	}

	def := map[string]any{}
	for k, v := range base {
		def[k] = v
	}
	got := applyWorkEnvelopeWallClockPolicy(false, def)
	if len(got) != 0 {
		t.Fatalf("default path is silent clamp (no diagnostic), got %#v", got)
	}
	if def[objects.FieldKeyActualEffort] != "2h" {
		t.Fatalf("default path should clamp in-place, got %v", def[objects.FieldKeyActualEffort])
	}

	over := map[string]any{}
	for k, v := range base {
		over[k] = v
	}
	got = applyWorkEnvelopeWallClockPolicy(true, over)
	if len(got) != 0 {
		t.Fatalf("break-glass skip should not warn or clamp, got %#v", got)
	}
	if over[objects.FieldKeyActualEffort] != "1d" {
		t.Fatalf("break-glass skip must leave overstated actual, got %v", over[objects.FieldKeyActualEffort])
	}

	policy := map[string]any{
		objects.FieldKeyKind:         objects.KindPolicy,
		objects.FieldKeyCreatedAt:    "2026-08-05T10:00:00Z",
		objects.FieldKeyUpdatedAt:    "2026-08-05T12:00:00Z",
		objects.FieldKeyActualEffort: "1d",
	}
	if got := applyWorkEnvelopeWallClockPolicy(false, policy); len(got) != 0 {
		t.Fatalf("non-effort_aware must not diagnose, got %#v", got)
	}
	if policy[objects.FieldKeyActualEffort] != "1d" {
		t.Fatalf("non-effort_aware must not clamp leftover actual, got %v", policy[objects.FieldKeyActualEffort])
	}
}
