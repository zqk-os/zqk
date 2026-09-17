package objectrecord

import (
	"context"
	"testing"
)

type fakeRecorder struct {
	created, updated, deleted []string
}

func (f *fakeRecorder) RecordObjectCreated(id string) { f.created = append(f.created, id) }
func (f *fakeRecorder) RecordObjectUpdated(id string) { f.updated = append(f.updated, id) }
func (f *fakeRecorder) RecordObjectDeleted(id string) { f.deleted = append(f.deleted, id) }

func TestFromContextRoundTrip(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	ctx := WithRecorder(context.Background(), rec)
	got := FromContext(ctx)
	if got != rec {
		t.Fatalf("FromContext = %v, want fakeRecorder", got)
	}
	if FromContext(context.Background()) != nil {
		t.Fatal("empty context should have nil recorder")
	}
}
