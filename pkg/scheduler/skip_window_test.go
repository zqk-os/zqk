package scheduler

import (
	"testing"
	"time"
)

func TestJobMatchesSchedulerSkipWindow(t *testing.T) {
	t.Parallel()
	until := time.Now().UTC().Add(time.Hour)
	sw := &SchedulerSkipWindow{Until: until, JobTypes: []string{"convergence_session_tick"}, TriggerTypes: []string{"timer"}}
	job := &ScheduledJob{ID: "SCH-1", JobType: "convergence_session_tick", TriggerType: "timer", Title: "tick"}
	if !JobMatchesSchedulerSkipWindow(job, sw) {
		t.Fatal("expected match")
	}
	job2 := &ScheduledJob{ID: "SCH-2", JobType: "run_wrapper", TriggerType: "timer"}
	if JobMatchesSchedulerSkipWindow(job2, sw) {
		t.Fatal("expected no match (job_type)")
	}
	sw2 := &SchedulerSkipWindow{Until: until, TitleContains: "Convergence"}
	job3 := &ScheduledJob{ID: "SCH-3", JobType: "convergence_session_tick", TriggerType: "timer", Title: "Convergence — test"}
	if !JobMatchesSchedulerSkipWindow(job3, sw2) {
		t.Fatal("expected title match")
	}
	swAll := &SchedulerSkipWindow{Until: until, MatchAll: true}
	if !JobMatchesSchedulerSkipWindow(job2, swAll) {
		t.Fatal("expected match_all")
	}
	swEmpty := &SchedulerSkipWindow{Until: until}
	if JobMatchesSchedulerSkipWindow(job, swEmpty) {
		t.Fatal("expected no match without filters or match_all")
	}
}

func TestLoadSchedulerSkipWindow_Expired(t *testing.T) {
	t.Parallel()
	d := t.TempDir()
	sw := &SchedulerSkipWindow{Until: time.Now().UTC().Add(-time.Minute), JobTypes: []string{"x"}}
	if err := SaveSchedulerSkipWindow(d, sw); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSchedulerSkipWindow(d, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil for expired window, got %+v", got)
	}
}
