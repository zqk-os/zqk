package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRecorder struct {
	createN, validN, dupN int
	lastSuccess           bool
	usedCAS               bool
}

func (f *fakeRecorder) RecordAuditEventCreation(ctx context.Context, eventType string, usedCAS bool, duration time.Duration, success bool) {
	f.createN++
	f.usedCAS = usedCAS
	f.lastSuccess = success
}

func (f *fakeRecorder) RecordAuditEventValidation(success bool) {
	f.validN++
	f.lastSuccess = success
}

func (f *fakeRecorder) RecordAuditEventDuplicate() {
	f.dupN++
}

func TestRecordPersistOutcomeDuplicate(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	RecordPersistOutcome(context.Background(), rec, EventTypeCacheInvalidation, true, time.Millisecond, PersistResult{
		Err:           errors.New("already exists"),
		AlreadyExists: true,
	})
	if rec.createN != 1 || rec.validN != 1 || rec.dupN != 1 || !rec.lastSuccess || !rec.usedCAS {
		t.Fatalf("%+v", rec)
	}
}

func TestRecordPersistOutcomeRetryOK(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	RecordPersistOutcome(context.Background(), rec, EventTypeCacheInvalidation, false, 0, PersistResult{Retried: true})
	if rec.createN != 1 || rec.validN != 1 || rec.dupN != 0 || !rec.lastSuccess {
		t.Fatalf("%+v", rec)
	}
}

func TestRecordBuffered(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	RecordBuffered(context.Background(), rec, EventTypeCacheInvalidation)
	if rec.createN != 1 || rec.validN != 0 || rec.dupN != 0 || !rec.lastSuccess || rec.usedCAS {
		t.Fatalf("%+v", rec)
	}
}
