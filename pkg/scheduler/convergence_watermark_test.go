package scheduler

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestShouldSkipConvergencePersistForDuplicateWatermark(t *testing.T) {
	snap := &TestBundleConvergenceSnapshot{HealthWatermarkRFC3339: "2026-04-07T12:00:00Z"}
	cvs := map[string]any{objects.FieldKeyLastMeasurementAt: "2026-04-07T12:00:00Z"}
	if !ShouldSkipConvergencePersistForDuplicateWatermark(cvs, snap) {
		t.Fatal("expected skip when watermark equals last_measurement_at")
	}
	cvs2 := map[string]any{objects.FieldKeyLastMeasurementAt: "2026-04-06T12:00:00Z"}
	if ShouldSkipConvergencePersistForDuplicateWatermark(cvs2, snap) {
		t.Fatal("expected no skip when watermark advanced")
	}
	if ShouldSkipConvergencePersistForDuplicateWatermark(nil, snap) {
		t.Fatal("empty CVS should not skip")
	}
	if ShouldSkipConvergencePersistForDuplicateWatermark(cvs, nil) {
		t.Fatal("nil snap should not skip")
	}
}

func TestConvergenceDuplicateWatermarkAuditUpdate(t *testing.T) {
	ts := time.Date(2026, 4, 10, 15, 30, 0, 0, time.UTC)
	cvs := map[string]any{
		objects.FieldKeyCurrentPhase:      "c5_verify",
		objects.FieldKeyLastMeasurementAt: "2026-04-07T12:00:00Z",
	}
	snap := &TestBundleConvergenceSnapshot{HealthWatermarkRFC3339: "2026-04-07T12:00:00Z"}
	upd := ConvergenceDuplicateWatermarkAuditUpdate(cvs, snap, ts)
	al, ok := upd[objects.FieldKeyActivityLog].([]any)
	if !ok || len(al) != 1 {
		t.Fatalf("expected one activity_log entry, got %#v", upd[objects.FieldKeyActivityLog])
	}
	row, ok := al[0].(map[string]any)
	if !ok {
		t.Fatalf("entry type: %#v", al[0])
	}
	if row[convSugKeyActivityAction] != convSugKeyActivityActionNoNewWatermark {
		t.Fatalf("action: %v", row[convSugKeyActivityAction])
	}
	if row[convSugKeyActivityTimestamp] != ts.UTC().Format(time.RFC3339) {
		t.Fatalf("timestamp: got %v want %s", row[convSugKeyActivityTimestamp], ts.UTC().Format(time.RFC3339))
	}
	if row[convSugKeyHealthWatermarkRFC3339] != snap.HealthWatermarkRFC3339 {
		t.Fatalf("health_watermark_rfc3339: %v", row[convSugKeyHealthWatermarkRFC3339])
	}
}
