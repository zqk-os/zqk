package scheduler

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// buildLastCompletionsMap builds a map of last completion times by job ID
func buildLastCompletionsMap(activityEvents []jobActivityEvent) map[string]time.Time {
	lastCompletions := make(map[string]time.Time)
	for _, event := range activityEvents {
		if event.EventType == "scheduler_job_completed" || event.EventType == "scheduler_job_failed" {
			if t, err := time.Parse(time.RFC3339, event.CreatedAt); err == nil {
				if lastCompletions[event.JobID].IsZero() || t.After(lastCompletions[event.JobID]) {
					lastCompletions[event.JobID] = t
				}
			}
		}
	}
	return lastCompletions
}

// shouldCheckJobForMissedTriggers checks if a job should be checked for missed triggers
func shouldCheckJobForMissedTriggers(job map[string]any) bool {
	triggerType := getString(job, "trigger_type")
	if triggerType != "timer" {
		return false
	}

	enabled := getBool(job, "enabled")
	if !enabled {
		return false
	}

	scheduleExpr := getString(job, "schedule_expression")
	return scheduleExpr != emptyValue
}

// estimateIntervalBetweenRuns returns the duration between two consecutive schedule ticks after now.
func estimateIntervalBetweenRuns(schedule cron.Schedule, now time.Time) time.Duration {
	n1 := schedule.Next(now.UTC())
	n2 := schedule.Next(n1)
	if n2.After(n1) {
		return n2.Sub(n1)
	}
	return 0
}

// missedTriggerAssessmentCodes are short CLI table keys; see outputMissedTriggersLegendToBuffer.
const (
	missedAssessmentHF = "HF" // high-frequency timer grace noise
	missedAssessmentFQ = "FQ" // frequent cadence, many gaps
	missedAssessmentST = "ST" // stale first gap (≥48h)
	missedAssessmentLR = "LR" // last completion before first missed slot
	missedAssessmentM  = "M"  // default / generic
)

// missedTriggerAssessmentParts returns a compact code for dense CLI output and a full sentence for JSON/YAML.
func missedTriggerAssessmentParts(missedCount int, firstMissed, lastRun time.Time, approxInterval time.Duration, now time.Time) (code string, detail string) {
	if missedCount <= 0 || firstMissed.IsZero() {
		return "", ""
	}
	late := now.Sub(firstMissed)
	if approxInterval > 0 {
		// High-frequency schedule: many "misses" may be grace-window noise
		if approxInterval < 2*time.Minute && missedCount >= 8 {
			return missedAssessmentHF, fmt.Sprintf("high-frequency timer (~%s between runs); %d gaps — often timing/grace, not necessarily outages",
				formatDurationShort(approxInterval), missedCount)
		}
		if approxInterval < 10*time.Minute && missedCount >= 15 {
			return missedAssessmentFQ, fmt.Sprintf("frequent schedule (~%s); many misses — check daemon load or widen grace if expected",
				formatDurationShort(approxInterval))
		}
	}
	if late >= 48*time.Hour && (approxInterval == 0 || approxInterval >= 12*time.Hour) {
		return missedAssessmentST, fmt.Sprintf("first missed slot was %s ago — worth investigating (daemon downtime or job errors)", formatDurationShort(late))
	}
	if !lastRun.IsZero() && lastRun.Before(firstMissed) {
		return missedAssessmentLR, fmt.Sprintf("%d missed expected run(s) since last successful completion", missedCount)
	}
	return missedAssessmentM, fmt.Sprintf("%d missed expected run(s); first gap at %s", missedCount, firstMissed.Format(time.RFC3339))
}

func formatDurationShort(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Round(time.Second)/time.Second))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Round(time.Minute)/time.Minute))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Round(time.Hour)/time.Hour))
	}
	return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
}

// parseCronSchedule parses a cron expression and returns a schedule
func parseCronSchedule(scheduleExpr string) (cron.Schedule, error) {
	return schedpkg.ParseCronSchedule(scheduleExpr)
}

// maxCronSlotsPerJob caps how many expected fire times we enumerate (safety bound for tight schedules).
const maxCronSlotsPerJob = 4096

// calculateExpectedRunTimes calculates expected fire times strictly after lastRun (or lookback) up to now.
func calculateExpectedRunTimes(schedule cron.Schedule, lastRun time.Time, now time.Time, jobNeverRan bool) []time.Time {
	if jobNeverRan {
		recentWindow := 24 * time.Hour
		lastRun = now.Add(-recentWindow)
	}

	expectedRuns := []time.Time{}
	checkTime := lastRun
	maxLookback := now.AddDate(0, 0, -7) // 7 days ago
	if !jobNeverRan && lastRun.Before(maxLookback) {
		checkTime = maxLookback
	}

	for i := 0; i < maxCronSlotsPerJob; i++ {
		nextRun := schedule.Next(checkTime)
		if nextRun.After(now) {
			break
		}
		expectedRuns = append(expectedRuns, nextRun)
		checkTime = nextRun
	}

	return expectedRuns
}

// checkForMissedTriggers checks if any expected fire times lack a completion in the half-open
// cron slot [expectedTime, nextFire). A completion shortly before the tick is ignored; one at
// 08:45 still counts for the 08:00 slot (unlike a fixed 5m grace after the tick only).
func checkForMissedTriggers(jobID string, schedule cron.Schedule, expectedRuns []time.Time, activityEvents []jobActivityEvent) (int, time.Time) {
	missedCount := 0
	var firstMissed time.Time

	for _, expectedTime := range expectedRuns {
		slotEnd := schedule.Next(expectedTime)
		if !slotEnd.After(expectedTime) {
			continue
		}
		if !hasCompletionInCronSlot(jobID, expectedTime, slotEnd, activityEvents) {
			missedCount++
			if firstMissed.IsZero() {
				firstMissed = expectedTime
			}
		}
	}

	return missedCount, firstMissed
}

// hasCompletionInCronSlot reports whether there is a terminal completion whose audit time falls in
// [slotStart, slotEnd) (UTC). Matches delayed runs that finish before the next cron fire.
func hasCompletionInCronSlot(jobID string, slotStart, slotEnd time.Time, activityEvents []jobActivityEvent) bool {
	slotStart = slotStart.UTC()
	slotEnd = slotEnd.UTC()
	for _, event := range activityEvents {
		if event.JobID != jobID {
			continue
		}
		if event.EventType != "scheduler_job_completed" && event.EventType != "scheduler_job_failed" {
			continue
		}
		if eventTime, err := time.Parse(time.RFC3339, event.CreatedAt); err == nil {
			et := eventTime.UTC()
			if !et.Before(slotStart) && et.Before(slotEnd) {
				return true
			}
		}
	}
	return false
}

// lastRunAtFromJob parses last_run_at from a scheduler_job map (storage). Returns zero time if missing or invalid.
func lastRunAtFromJob(job map[string]any) time.Time {
	s := getString(job, "last_run_at")
	if s == emptyValue {
		return time.Time{}
	}
	// Daemon writes "2006-01-02T15:04:05Z"; RFC3339 accepts that and other variants
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05Z", s)
		if err != nil {
			return time.Time{}
		}
	}
	return t.UTC()
}

// processJobForMissedTriggers processes a single job for missed triggers.
// Uses the later of activity-derived last completion and job's last_run_at from storage, so that
// when the job has run successfully since a missed window (e.g. daemon was down, then restarted
// and ran the job), we zero out missed count and displays stay consistent.
func processJobForMissedTriggers(jobID string, job map[string]any, lastCompletions map[string]time.Time, activityEvents []jobActivityEvent, now time.Time) *missedTrigger {
	scheduleExpr := getString(job, "schedule_expression")
	schedule, err := parseCronSchedule(scheduleExpr)
	if err != nil {
		return nil
	}

	lastRunFromActivity := lastCompletions[jobID]
	lastRunFromStorage := lastRunAtFromJob(job)
	effectiveLastRun := lastRunFromActivity
	if lastRunFromStorage.After(effectiveLastRun) {
		effectiveLastRun = lastRunFromStorage
	}
	jobNeverRan := effectiveLastRun.IsZero()
	approxInterval := estimateIntervalBetweenRuns(schedule, effectiveLastRun)

	// If we have a recent completion (from activity or storage) and we're still within one schedule interval,
	// do not report misses yet. This avoids false positives when a job ran off exact cron boundary and
	// activity events are sparse/rotated.
	if !jobNeverRan && approxInterval > 0 && now.Before(effectiveLastRun.Add(approxInterval)) {
		return nil
	}

	expectedRuns := calculateExpectedRunTimes(schedule, effectiveLastRun, now, jobNeverRan)
	missedCount, firstMissed := checkForMissedTriggers(jobID, schedule, expectedRuns, activityEvents)

	if missedCount > 0 {
		if approxInterval <= 0 {
			approxInterval = estimateIntervalBetweenRuns(schedule, now)
		}
		code, detail := missedTriggerAssessmentParts(missedCount, firstMissed, effectiveLastRun, approxInterval, now)
		mt := &missedTrigger{
			JobID:             jobID,
			ExpectedAt:        firstMissed,
			LastRun:           effectiveLastRun,
			MissedCount:       missedCount,
			ScheduleExpr:      scheduleExpr,
			ApproxIntervalSec: int64(approxInterval.Seconds()),
			ContextNote:       "Timer: expected fires from cron within ~7d lookback; completion must fall in [tick, next tick) vs audit events",
			AssessmentCode:    code,
			Assessment:        detail,
		}
		return mt
	}

	return nil
}
