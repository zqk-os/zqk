package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

type fakeSink struct {
	usedCAS bool
	err     error
	calls   int
	lastID  string
}

func (f *fakeSink) WriteSummary(ctx context.Context, dir, id string, event map[string]any, tags map[string]string) FlushWriteResult {
	f.calls++
	f.lastID = id
	return FlushWriteResult{UsedCAS: f.usedCAS, Err: f.err}
}

func TestWriteFlushRecords(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	sink := &fakeSink{usedCAS: true}
	got := WriteFlush(context.Background(), sink, rec, "/dir", "AUD-1", map[string]any{objects.FieldKeyID: "AUD-1"}, nil, true)
	if got.Err != nil || !got.UsedCAS || sink.calls != 1 || sink.lastID != "AUD-1" {
		t.Fatalf("%+v calls=%d id=%s", got, sink.calls, sink.lastID)
	}
	if rec.createN != 1 || rec.validN != 1 || !rec.lastSuccess || !rec.usedCAS {
		t.Fatalf("rec=%+v", rec)
	}
}

func TestWriteFlushNilSink(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	got := WriteFlush(context.Background(), nil, rec, "", "", nil, nil, false)
	if got.Err != nil || rec.createN != 1 || rec.lastSuccess {
		t.Fatalf("%+v rec=%+v", got, rec)
	}
}

func TestWriteFlushError(t *testing.T) {
	t.Parallel()
	want := errors.New("disk")
	rec := &fakeRecorder{}
	got := WriteFlush(context.Background(), &fakeSink{err: want}, rec, "/d", "AUD-2", nil, nil, true)
	if !errors.Is(got.Err, want) || rec.createN != 1 || rec.validN != 1 || !rec.lastSuccess {
		t.Fatalf("%+v rec=%+v", got, rec)
	}
}
