package scheduler

import (
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ShouldSkipConvergencePersistForDuplicateWatermark returns true when applying the current
// health snapshot would not advance last_measurement_at — the same rule as
// convergence_session_tick (no duplicate writes for identical health.jsonl watermark).
func ShouldSkipConvergencePersistForDuplicateWatermark(cvs map[string]any, snap *TestBundleConvergenceSnapshot) bool {
	if snap == nil || snap.HealthWatermarkRFC3339 == "" {
		return false
	}
	prev := FieldAsString(cvs[objects.FieldKeyLastMeasurementAt])
	return prev != "" && prev == snap.HealthWatermarkRFC3339
}

// ConvergenceDuplicateWatermarkAuditUpdate returns a minimal object update (activity_log append only)
// so a measure run is recorded when the health tail watermark matches CVS last_measurement_at.
func ConvergenceDuplicateWatermarkAuditUpdate(cvs map[string]any, snap *TestBundleConvergenceSnapshot, wallClock time.Time) map[string]any {
	phase := ""
	if cvs != nil {
		phase = FieldAsString(cvs[objects.FieldKeyCurrentPhase])
	}
	return map[string]any{
		objects.FieldKeyActivityLog: []any{
			BuildConvergenceActivityLogEntryNoNewWatermark(phase, snap, wallClock),
		},
	}
}
