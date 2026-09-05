package storage

import (
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// TRACK: BLI-KERNEL-WORK-ENVELOPE-001 — work-envelope defaults key off
// KindHasTrait + lifecycle work_done / execution_locked. Lift effort off
// base_object remains CRIT-KERNEL-WORK-ENVELOPE-BASE-LIFT-001.

func kindHasNamedTrait(kind, trait string) bool {
	return objects.KindHasNamedTrait(kind, trait)
}

func kindStatusChecker() objects.IStatusChecker {
	return objects.GetGlobalStatusChecker()
}

// applyCompleteTransitionDefaults fills work-envelope fields on status hops
// before validation. Returns true if existing was mutated.
//
// → execution-locked (completable): started_at if empty.
// → work_done (completable): completed_at if empty; started_at from created_at
// if still empty. effort_aware actual_effort is the started_at→completed_at
// span (created_at fallback), never a copy of estimated_effort.
func applyCompleteTransitionDefaults(kind string, existing map[string]any, oldStatus, newStatus string) bool {
	if existing == nil {
		return false
	}
	newStatus = strings.TrimSpace(newStatus)
	oldStatus = strings.TrimSpace(oldStatus)
	if newStatus == emptyValue || strings.EqualFold(newStatus, oldStatus) {
		return false
	}
	completable := kindHasNamedTrait(kind, "completable")
	if !completable {
		return false
	}
	sc := kindStatusChecker()
	mutated := false
	if sc.Role(kind, newStatus) == objects.LifecycleRoleExecutionLocked {
		if ensureStartedAt(existing, objects.FieldKeyUpdatedAt) {
			mutated = true
		}
	}
	if !sc.IsWorkDone(kind, newStatus) {
		return mutated
	}
	if strings.TrimSpace(objects.GetString(existing, objects.FieldKeyStartedAt)) == emptyValue &&
		strings.TrimSpace(objects.GetString(existing, objects.FieldKeyCreatedAt)) != emptyValue {
		if ensureStartedAt(existing, objects.FieldKeyCreatedAt) {
			mutated = true
		}
	}
	if ensureCompletedAtOnComplete(existing) {
		mutated = true
	}
	if kindHasNamedTrait(kind, "effort_aware") && applyCompleteActualEffort(existing) {
		mutated = true
	}
	return mutated
}

// CompletedAtBackfillStamp returns historical updated_at as completed_at when an
// effort_aware object is already work-done and the work clock was never stamped.
// One-shot backfill only — do not call from object get.
func CompletedAtBackfillStamp(kind string, obj map[string]any) (stamp string, ok bool) {
	if obj == nil || !kindHasNamedTrait(kind, "effort_aware") {
		return "", false
	}
	status := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyStatus))
	if !kindStatusChecker().IsWorkDone(kind, status) {
		return "", false
	}
	if strings.TrimSpace(objects.GetString(obj, objects.FieldKeyCompletedAt)) != emptyValue {
		return "", false
	}
	updated := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyUpdatedAt))
	if updated == emptyValue {
		return "", false
	}
	return objectDateTimeStampZ(updated), true
}

func ensureStartedAt(obj map[string]any, preferKey string) bool {
	if strings.TrimSpace(objects.GetString(obj, objects.FieldKeyStartedAt)) != emptyValue {
		return false
	}
	obj[objects.FieldKeyStartedAt] = objectDateTimeStampZ(objects.GetString(obj, preferKey))
	return true
}

func ensureCompletedAtOnComplete(obj map[string]any) bool {
	if strings.TrimSpace(objects.GetString(obj, objects.FieldKeyCompletedAt)) != emptyValue {
		return false
	}
	obj[objects.FieldKeyCompletedAt] = objectDateTimeStampZ(objects.GetString(obj, objects.FieldKeyUpdatedAt))
	return true
}

func objectDateTimeStampZ(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != emptyValue {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return zqktime.FormatLayoutUTC(t, zqktime.LayoutObjectDateTimeZ)
		}
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return zqktime.FormatLayoutUTC(t, zqktime.LayoutObjectDateTimeZ)
		}
	}
	return zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
}

func applyCompleteActualEffort(obj map[string]any) bool {
	if actualEffortIsSet(obj) {
		clamped, _, _ := validation.ClampActualEffortToWallClock(obj)
		return clamped
	}
	if span, ok := validation.WallClockActualEffortString(obj); ok {
		obj[objects.FieldKeyActualEffort] = span
		return true
	}
	// Spec requires the field on complete; prefer a stable placeholder over
	// leaving Tier-1 standing debt when timestamps cannot yield a span.
	obj[objects.FieldKeyActualEffort] = "unspecified"
	return true
}

func actualEffortIsSet(obj map[string]any) bool {
	v, ok := obj[objects.FieldKeyActualEffort]
	return ok && effortValuePresent(v)
}

func effortValuePresent(v any) bool {
	if v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != emptyValue
	default:
		return true
	}
}
