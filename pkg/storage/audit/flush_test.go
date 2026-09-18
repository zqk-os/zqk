package audit

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestFlushActor(t *testing.T) {
	t.Parallel()
	if FlushActor("") != ActorSystem {
		t.Fatalf("empty: %q", FlushActor(""))
	}
	if FlushActor("ACC-1") != "ACC-1" {
		t.Fatalf("id: %q", FlushActor("ACC-1"))
	}
	if FlushActorFromSec(nil) != ActorSystem {
		t.Fatal("nil sec")
	}
	if got := FlushActorFromSec(&pkgctx.SecurityContext{AccountID: "ACC-2"}); got != "ACC-2" {
		t.Fatalf("sec: %q", got)
	}
}

func TestPrepareFlush(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g := &Group{EventType: EventTypeCacheInvalidation, Severity: SeverityLow, Count: 3, FirstSeen: now, LastSeen: now}
	ids := &fakeIDs{ids: []string{"AUD-9"}}
	plan, err := PrepareFlush(ids, "ACC-1", "k", g, now)
	if err != nil || plan.ID != "AUD-9" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if objects.GetString(plan.Event, objects.FieldKeyEventType) != EventTypeAggregatedSummary {
		t.Fatalf("event_type=%v", plan.Event[objects.FieldKeyEventType])
	}
	if objects.GetString(plan.Event, objects.FieldKeyCreatedBy) != "ACC-1" {
		t.Fatalf("actor=%v", plan.Event[objects.FieldKeyCreatedBy])
	}
}

func TestPrepareFlushNil(t *testing.T) {
	t.Parallel()
	plan, err := PrepareFlush(nil, "", "k", &Group{}, time.Now())
	if err != nil || plan.Event != nil {
		t.Fatalf("%+v %v", plan, err)
	}
}

func TestFlushMetricTags(t *testing.T) {
	t.Parallel()
	g := &Group{EventType: "cache_update", TargetKind: "backlog_item", Severity: "low"}
	tags := FlushMetricTags(g)
	if tags[objects.FieldKeyEventType] != "cache_update" || tags[objects.FieldKeyTargetKind] != "backlog_item" {
		t.Fatalf("%v", tags)
	}
}

func TestRecordFlushWrite(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	RecordFlushWrite(context.Background(), rec, true, time.Millisecond, true, false)
	if rec.createN != 1 || rec.validN != 1 || rec.lastSuccess || !rec.usedCAS {
		t.Fatalf("%+v", rec)
	}
}

func TestPrepareFlushIDError(t *testing.T) {
	t.Parallel()
	_, err := PrepareFlush(&fakeIDs{}, "", "k", &Group{EventType: "x"}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected id error")
	}
}

func TestFlushRunWritesViaSink(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	g := &Group{EventType: EventTypeCacheInvalidation, Severity: SeverityLow, Count: 2, FirstSeen: now, LastSeen: now}
	sink := &fakeSink{usedCAS: true}
	rec := &fakeRecorder{}
	plan, _, written, err := FlushRun(
		context.Background(),
		context.Background(),
		"/audit",
		func(context.Context, string) IDAllocator { return &fakeIDs{ids: []string{"AUD-7"}} },
		sink,
		rec,
		"ACC-1",
		"k",
		g,
		now,
	)
	if err != nil || plan.ID != "AUD-7" || written.Err != nil || !written.UsedCAS || sink.lastID != "AUD-7" {
		t.Fatalf("plan=%+v written=%+v err=%v last=%s", plan, written, err, sink.lastID)
	}
	if rec.createN != 1 {
		t.Fatalf("rec=%+v", rec)
	}
}

func TestFlushRunIDErrorSkipsWrite(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	_, _, written, err := FlushRun(
		context.Background(),
		context.Background(),
		"/audit",
		func(context.Context, string) IDAllocator { return &fakeIDs{} },
		sink,
		nil,
		"",
		"k",
		&Group{EventType: "x"},
		time.Now().UTC(),
	)
	if err == nil || sink.calls != 0 || written.Err != nil {
		t.Fatalf("err=%v calls=%d written=%+v", err, sink.calls, written)
	}
}
