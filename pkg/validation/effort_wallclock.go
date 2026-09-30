package validation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// RuleActualEffortWallClock is a detector for actual_effort above the work span.
// It is not a persist-blocking field error: the transition membrane clamps, and
// lifecycle break-glass skips clamp the same way it skips auto-only status edges.
const RuleActualEffortWallClock = "actual_effort_wall_clock"

// RuleActualEffortWallClockClamp is the advisory rewrite warning after clamp.
const RuleActualEffortWallClockClamp = "actual_effort_wall_clock_clamp"

var (
	effortTokenRE = regexp.MustCompile(`(?i)^\s*([0-9]*\.?[0-9]+)\s*(h|hr|hrs|hour|hours|d|day|days|w|wk|week|weeks|m|min|mins|minute|minutes)?\s*$`)
	effortRangeRE = regexp.MustCompile(`(?i)^\s*(.+?)\s*[-–—]\s*(.+?)\s*$`)
)

// ParseEffortHours parses human effort strings (e.g. "4h", "1d", "0.5d", "30m", "2-3 days").
// Qualitative tokens (small/medium/large) return ok=false.
// Ranges use the upper bound.
func ParseEffortHours(raw string) (hours float64, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	lower := strings.ToLower(s)
	switch lower {
	case "small", "medium", "large", "xl", "xs", "tiny", "huge":
		return 0, false
	}
	if m := effortRangeRE.FindStringSubmatch(s); m != nil {
		_, okLo := ParseEffortHours(m[1])
		hi, okHi := ParseEffortHours(m[2])
		if okHi {
			return hi, true
		}
		if okLo {
			lo, _ := ParseEffortHours(m[1])
			return lo, true
		}
		return 0, false
	}
	m := effortTokenRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil || n < 0 {
		return 0, false
	}
	unit := strings.ToLower(m[2])
	switch unit {
	case "", "h", "hr", "hrs", "hour", "hours":
		return n, true
	case "m", "min", "mins", "minute", "minutes":
		return n / 60.0, true
	case "d", "day", "days":
		return n * 24, true
	case "w", "wk", "week", "weeks":
		return n * 24 * 7, true
	default:
		return 0, false
	}
}

func parseObjectTime(obj map[string]any, key string) (time.Time, bool) {
	if obj == nil {
		return time.Time{}, false
	}
	raw, _ := obj[key].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func effortWallSpanHours(obj map[string]any) (wallH float64, ok bool, inverted bool) {
	start, okS := parseObjectTime(obj, objects.FieldKeyStartedAt)
	if !okS {
		start, okS = parseObjectTime(obj, objects.FieldKeyCreatedAt)
	}
	if !okS {
		return 0, false, false
	}
	end, okE := parseObjectTime(obj, objects.FieldKeyCompletedAt)
	if !okE {
		end, okE = parseObjectTime(obj, objects.FieldKeyUpdatedAt)
	}
	if !okE {
		return 0, false, false
	}
	if !end.After(start) && !end.Equal(start) {
		return 0, true, true
	}
	return end.Sub(start).Hours(), true, false
}

// FormatEffortHours formats a non-negative hour count as an effort string (e.g. "32h").
func FormatEffortHours(hours float64) string {
	if hours < 0 {
		hours = 0
	}
	// Prefer whole hours when within 1 minute of an integer.
	rounded := float64(int(hours + 1e-9))
	if hours-rounded < 1.0/60.0 {
		return fmt.Sprintf("%dh", int(rounded))
	}
	return fmt.Sprintf("%.2fh", hours)
}

// formatEffortHoursFloor formats a wall-clock span conservatively (never above wall).
func formatEffortHoursFloor(hours float64) string {
	if hours < 0 {
		hours = 0
	}
	whole := int(hours) // toward zero for positive spans
	if whole == 0 && hours > 0 {
		return FormatEffortHours(hours)
	}
	return fmt.Sprintf("%dh", whole)
}

// WallClockActualEffortString formats started_at (created_at fallback) →
// completed_at/updated_at as an effort token (same rounding as
// ClampActualEffortToWallClock). ok is false when the span cannot be computed.
func WallClockActualEffortString(obj map[string]any) (string, bool) {
	wallH, okSpan, inverted := effortWallSpanHours(obj)
	if !okSpan {
		return "", false
	}
	if inverted {
		return "0h", true
	}
	return formatEffortHoursFloor(wallH), true
}

// ClampActualEffortToWallClock rewrites actual_effort down to the wall-clock span when
// it exceeds started_at (created_at fallback) → completed/updated elapsed time.
// Qualitative / unparsable values are left unchanged. Returns whether a rewrite occurred.
// TRACK: stop estimated→actual copy from failing validation forever.
func ClampActualEffortToWallClock(obj map[string]any) (clamped bool, previous, next string) {
	if obj == nil {
		return false, "", ""
	}
	raw, _ := obj[objects.FieldKeyActualEffort].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, "", ""
	}
	actualH, ok := ParseEffortHours(raw)
	if !ok || actualH <= 0 {
		return false, "", ""
	}
	wallH, okSpan, inverted := effortWallSpanHours(obj)
	if !okSpan {
		return false, "", ""
	}
	if inverted {
		next = "0h"
		if raw == next {
			return false, raw, next
		}
		obj[objects.FieldKeyActualEffort] = next
		return true, raw, next
	}
	// 1 minute slack matches leftover detection after clamp.
	if actualH <= wallH+(1.0/60.0) {
		return false, raw, raw
	}
	next = formatEffortHoursFloor(wallH)
	obj[objects.FieldKeyActualEffort] = next
	return true, raw, next
}

// ValidateActualEffortWithinWallClock reports when actual_effort exceeds the work
// span. Callers must treat the result as a warning (or ignore it after clamp):
// process admins cannot "fix" this field — the membrane autofills/clamps, and
// only lifecycle override / break-glass may persist an overstated actual.
func ValidateActualEffortWithinWallClock(obj map[string]any) []ValidationWarning {
	if obj == nil {
		return nil
	}
	if !objects.KindHasNamedTrait(objects.GetString(obj, objects.FieldKeyKind), "effort_aware") {
		return nil
	}
	raw, _ := obj[objects.FieldKeyActualEffort].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	actualH, ok := ParseEffortHours(raw)
	if !ok {
		return nil
	}
	wallH, okSpan, inverted := effortWallSpanHours(obj)
	if !okSpan {
		return nil
	}
	if inverted {
		if actualH > 0 {
			return []ValidationWarning{{
				Field:   objects.FieldKeyActualEffort,
				Message: fmt.Sprintf("actual_effort %q exceeds wall-clock span (updated/completed before started_at/created_at)", raw),
				Rule:    RuleActualEffortWallClock,
			}}
		}
		return nil
	}
	if actualH > wallH+(1.0/60.0) {
		return []ValidationWarning{{
			Field: objects.FieldKeyActualEffort,
			Message: fmt.Sprintf(
				"actual_effort %q (%.2fh) exceeds wall-clock from started_at/created_at to updated/completed (%.2fh); membrane should have clamped",
				raw, actualH, wallH,
			),
			Rule: RuleActualEffortWallClock,
		}}
	}
	return nil
}

// applyWorkEnvelopeWallClockPolicy clamps overstated actual_effort on the default
// path with no user-facing diagnostic (POL-CODE-ACTIONABLE-DIAGNOSTICS-001).
// skipOverride is the same gate as auto-only status edges (break-glass / trusted
// shockwave): do not clamp — the override may persist.
func applyWorkEnvelopeWallClockPolicy(skipOverride bool, obj map[string]any) []ValidationWarning {
	if skipOverride || obj == nil {
		return nil
	}
	if !objects.KindHasNamedTrait(objects.GetString(obj, objects.FieldKeyKind), "effort_aware") {
		return nil
	}
	_, _, _ = ClampActualEffortToWallClock(obj)
	return nil
}
