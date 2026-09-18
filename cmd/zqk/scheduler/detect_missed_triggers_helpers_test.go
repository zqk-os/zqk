package scheduler

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/robfig/cron/v3"
)

func TestLastRunAtFromJob(t *testing.T) {
	t.Parallel()
	// RFC3339 format (daemon and storage)
	job1 := map[string]any{objects.FieldKeyID: "SCH-cache-prewarm", objects.FieldKeyLastRunAt: "2026-03-13T23:40:00Z"}
	if got := lastRunAtFromJob(job1); got.Year() != 2026 || got.Hour() != 23 || got.Minute() != 40 {
		t.Errorf("lastRunAtFromJob(RFC3339) = %v, want 2026-03-13 23:40 UTC", got)
	}
	// Alternative format
	job2 := map[string]any{objects.FieldKeyID: "SCH-maintenance-wal", objects.FieldKeyLastRunAt: "2006-01-02T15:04:05Z"}
	if got := lastRunAtFromJob(job2); got.Year() != 2006 || got.Hour() != 15 {
		t.Errorf("lastRunAtFromJob(2006-01-02T15:04:05Z) = %v", got)
	}
	// Missing or empty
	if got := lastRunAtFromJob(map[string]any{objects.FieldKeyID: "X"}); !got.IsZero() {
		t.Errorf("lastRunAtFromJob(no last_run_at) = %v, want zero", got)
	}
	if got := lastRunAtFromJob(map[string]any{objects.FieldKeyLastRunAt: ""}); !got.IsZero() {
		t.Errorf("lastRunAtFromJob(empty last_run_at) = %v, want zero", got)
	}
}

// TestProcessJobForMissedTriggers_ZeroOutWhenJobHasLastRunAt verifies that when the job has
// last_run_at from storage (e.g. daemon updated it after a successful run), we do not count
// that period as missed even if activity events were cleared or are empty.
func TestProcessJobForMissedTriggers_ZeroOutWhenJobHasLastRunAt(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	// Job ran 2 minutes ago (per storage); schedule every 10 min. Next expected run is in ~8 min, so no expected runs before now → 0 missed.
	lastRun := now.Add(-2 * time.Minute)
	job := map[string]any{
		objects.FieldKeyID:                 "SCH-cache-prewarm",
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyScheduleExpression: "0 */10 * * * *",
		objects.FieldKeyLastRunAt:          lastRun.Format("2006-01-02T15:04:05Z"),
	}
	lastCompletions := make(map[string]time.Time) // no activity events
	var activityEvents []jobActivityEvent

	trigger := processJobForMissedTriggers("SCH-cache-prewarm", job, lastCompletions, activityEvents, now)
	if trigger != nil {
		t.Errorf("Expected no missed trigger when job last_run_at is recent (2 min ago); got MissedCount=%d LastRun=%v",
			trigger.MissedCount, trigger.LastRun)
	}
}

// TestProcessJobForMissedTriggers_StillMissedWhenNoRunAfterExpected verifies we still report missed
// when there is no completion after the expected run time (activity and storage both before expected).
func TestProcessJobForMissedTriggers_StillMissedWhenNoRunAfterExpected(t *testing.T) {
	t.Parallel()
	// Fix now so schedule is deterministic: 12:00. Schedule every 10 min. Last run at 11:30 → expected at 11:40, 11:50, 12:00.
	// So we have 3 expected runs. No completion events → 3 missed.
	base := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	lastRun := base.Add(-30 * time.Minute)
	job := map[string]any{
		objects.FieldKeyID:                 "SCH-cache-prewarm",
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyScheduleExpression: "0 */10 * * * *",
		objects.FieldKeyLastRunAt:          lastRun.Format("2006-01-02T15:04:05Z"),
	}
	lastCompletions := map[string]time.Time{"SCH-cache-prewarm": lastRun}
	var activityEvents []jobActivityEvent

	trigger := processJobForMissedTriggers("SCH-cache-prewarm", job, lastCompletions, activityEvents, base)
	if trigger == nil {
		t.Fatal("Expected missed triggers when no completion after last run")
	}
	if trigger.MissedCount < 1 {
		t.Errorf("Expected at least 1 missed, got %d", trigger.MissedCount)
	}
}

// TestProcessJobForMissedTriggers_EffectiveLastRunUsesStorageWhenNewer verifies that when
// storage last_run_at is newer than activity-derived last completion, we use storage and
// can zero out missed (next expected run is in the future).
func TestProcessJobForMissedTriggers_EffectiveLastRunUsesStorageWhenNewer(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	lastRunStorage := now.Add(-1 * time.Minute)
	lastRunActivity := now.Add(-30 * time.Minute)
	job := map[string]any{
		objects.FieldKeyID:                 "SCH-maintenance-wal",
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyScheduleExpression: "0 */15 * * * *",
		objects.FieldKeyLastRunAt:          lastRunStorage.Format("2006-01-02T15:04:05Z"),
	}
	lastCompletions := map[string]time.Time{"SCH-maintenance-wal": lastRunActivity}
	var activityEvents []jobActivityEvent

	trigger := processJobForMissedTriggers("SCH-maintenance-wal", job, lastCompletions, activityEvents, now)
	if trigger != nil {
		t.Errorf("Expected no missed trigger when storage last_run_at is newer than activity; got MissedCount=%d",
			trigger.MissedCount)
	}
}

// TestProcessJobForMissedTriggers_LateCompletionInSlotNotMissed verifies a completion more than 5m
// after the cron tick still satisfies that slot (half-open [tick, next tick)).
func TestProcessJobForMissedTriggers_LateCompletionInSlotNotMissed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 12, 11, 15, 0, 0, time.UTC)
	lastRun := now.Add(-10 * time.Minute) // 11:05 — only one cron fire (11:10) falls before now
	// 7 minutes after the 11:10 tick, still inside [11:10, 11:20)
	completionTime := now.Add(2 * time.Minute)
	job := map[string]any{
		objects.FieldKeyID:                 "SCH-pre-commit-lint",
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyScheduleExpression: "*/10 * * * *",
		objects.FieldKeyLastRunAt:          lastRun.Format("2006-01-02T15:04:05Z"),
	}
	lastCompletions := map[string]time.Time{"SCH-pre-commit-lint": lastRun}
	activityEvents := []jobActivityEvent{
		{JobID: "SCH-pre-commit-lint", EventType: "scheduler_job_completed", CreatedAt: completionTime.Format(time.RFC3339)},
	}

	trigger := processJobForMissedTriggers("SCH-pre-commit-lint", job, lastCompletions, activityEvents, now)
	if trigger != nil {
		t.Fatalf("expected no missed triggers when completion falls in slot after tick; got %d missed: %+v",
			trigger.MissedCount, trigger)
	}
}

func TestCheckForMissedTriggers_CronSlotBoundary(t *testing.T) {
	t.Parallel()
	specParser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	sched, err := specParser.Parse("0 */10 * * * *")
	if err != nil {
		t.Fatal(err)
	}
	slotStart := time.Date(2026, 4, 12, 11, 10, 0, 0, time.UTC)
	slotEnd := sched.Next(slotStart)
	evOK := []jobActivityEvent{{JobID: "J", EventType: "scheduler_job_completed", CreatedAt: "2026-04-12T11:17:00Z"}}
	if !hasCompletionInCronSlot("J", slotStart, slotEnd, evOK) {
		t.Error("11:17 completion should be inside [11:10,11:20)")
	}
	evLate := []jobActivityEvent{{JobID: "J", EventType: "scheduler_job_completed", CreatedAt: "2026-04-12T11:21:00Z"}}
	if hasCompletionInCronSlot("J", slotStart, slotEnd, evLate) {
		t.Error("11:21 completion should not count for 11:10 slot")
	}
}

func TestMissedTriggerAssessmentParts(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	t.Run("empty_when_no_miss", func(t *testing.T) {
		t.Parallel()
		code, detail := missedTriggerAssessmentParts(0, time.Time{}, time.Time{}, 0, now)
		if code != "" || detail != "" {
			t.Fatalf("want empty, got code=%q detail=%q", code, detail)
		}
	})

	t.Run("HF_high_frequency", func(t *testing.T) {
		t.Parallel()
		first := now.Add(-5 * time.Minute)
		code, _ := missedTriggerAssessmentParts(8, first, first.Add(time.Hour), time.Minute, now)
		if code != missedAssessmentHF {
			t.Fatalf("want %s, got %s", missedAssessmentHF, code)
		}
	})

	t.Run("FQ_frequent_cadence", func(t *testing.T) {
		t.Parallel()
		first := now.Add(-30 * time.Minute)
		code, _ := missedTriggerAssessmentParts(15, first, first.Add(time.Hour), 5*time.Minute, now)
		if code != missedAssessmentFQ {
			t.Fatalf("want %s, got %s", missedAssessmentFQ, code)
		}
	})

	t.Run("ST_stale_gap", func(t *testing.T) {
		t.Parallel()
		first := now.Add(-72 * time.Hour)
		code, _ := missedTriggerAssessmentParts(1, first, time.Time{}, 24*time.Hour, now)
		if code != missedAssessmentST {
			t.Fatalf("want %s, got %s", missedAssessmentST, code)
		}
	})

	t.Run("LR_last_run_before_first_miss", func(t *testing.T) {
		t.Parallel()
		first := now.Add(-5 * time.Hour)
		last := now.Add(-10 * time.Hour)
		code, _ := missedTriggerAssessmentParts(3, first, last, time.Hour, now)
		if code != missedAssessmentLR {
			t.Fatalf("want %s, got %s", missedAssessmentLR, code)
		}
	})

	t.Run("M_default", func(t *testing.T) {
		t.Parallel()
		first := now.Add(-10 * time.Minute)
		last := first.Add(time.Hour)
		code, _ := missedTriggerAssessmentParts(2, first, last, 30*time.Minute, now)
		if code != missedAssessmentM {
			t.Fatalf("want %s, got %s", missedAssessmentM, code)
		}
	})
}
