package system

import (
	"errors"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
)

type recordingCheckCallback struct {
	concurrency.NoOpOperationCallback
	completeID string
	duration   time.Duration
	completes  int
	errorID    string
	errors     int
}

func (r *recordingCheckCallback) OnComplete(operationID string, _ any, duration time.Duration) {
	r.completes++
	r.completeID = operationID
	r.duration = duration
}

func (r *recordingCheckCallback) OnError(operationID string, _ error) {
	r.errors++
	r.errorID = operationID
}

func TestReportSystemCheckCallback_CompleteUsesWallDuration(t *testing.T) {
	t.Parallel()
	rec := &recordingCheckCallback{}
	want := 42 * time.Second
	reportSystemCheckCallback(rec, "check_1788044408607551000", nil, want)

	if rec.completes != 1 {
		t.Fatalf("complete events = %d, want 1", rec.completes)
	}
	if rec.duration != want {
		t.Fatalf("duration = %v, want %v (must not log duration_seconds=0)", rec.duration, want)
	}
	if rec.errors != 0 {
		t.Fatalf("error events = %d, want 0", rec.errors)
	}
}

func TestReportSystemCheckCallback_ErrorSkipsComplete(t *testing.T) {
	t.Parallel()
	rec := &recordingCheckCallback{}
	reportSystemCheckCallback(rec, "check_err", errors.New("check failed"), 5*time.Second)

	if rec.completes != 0 {
		t.Fatalf("complete events = %d, want 0 on error", rec.completes)
	}
	if rec.errors != 1 {
		t.Fatalf("error events = %d, want 1", rec.errors)
	}
	if rec.errorID != "check_err" {
		t.Fatalf("operation id = %q, want check_err", rec.errorID)
	}
}

func TestReportSystemCheckCallback_NilCallback(t *testing.T) {
	t.Parallel()
	reportSystemCheckCallback(nil, "check_nil", nil, time.Second)
}
