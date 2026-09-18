package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
)

type fakeBuffer struct {
	aggregate bool
	addErr    error
	added     int
}

func (f *fakeBuffer) ShouldAggregate(event map[string]any) bool { return f.aggregate }

func (f *fakeBuffer) AddEvent(event map[string]any) error {
	f.added++
	return f.addErr
}

type fakeIDs struct {
	ids []string
	n   int
}

func (f *fakeIDs) GenerateNextID() (string, error) {
	if f.n >= len(f.ids) {
		return "", errors.New("no more ids")
	}
	id := f.ids[f.n]
	f.n++
	return id, nil
}

func testBuilder(t *testing.T, id string) *bldr_instance_v1.AuditEventInstanceBuilder {
	t.Helper()
	b := bldr_instance_v1.NewAuditEventInstanceBuilder("2.0.0")
	PopulateEvent(b, id, "/tmp/proj", "system", "2026-01-01T00:00:00Z", &EventOptions{
		EventType: EventTypeCacheInvalidation,
		Operation: "test",
		Severity:  SeverityLow,
	})
	return b
}

func TestDispatchBuffers(t *testing.T) {
	t.Parallel()
	buf := &fakeBuffer{aggregate: true}
	store := &fakeStore{errs: []error{errors.New("store should not run")}}
	got := Dispatch(context.Background(), nil, testBuilder(t, "AUD-1"), "AUD-1", &EventOptions{
		EventType: EventTypeCacheInvalidation,
		Severity:  SeverityLow,
	}, DispatchDeps{Store: store, Buffer: buf, Metrics: &fakeRecorder{}})
	if !got.Buffered || buf.added != 1 || store.calls != 0 {
		t.Fatalf("%+v added=%d calls=%d", got, buf.added, store.calls)
	}
}

func TestDispatchBufferAddFailsPersists(t *testing.T) {
	t.Parallel()
	buf := &fakeBuffer{aggregate: true, addErr: errors.New("busy")}
	store := &fakeStore{}
	opts := &EventOptions{EventType: EventTypeCacheInvalidation, Operation: "test", Severity: SeverityLow}
	got := Dispatch(context.Background(), nil, testBuilder(t, "AUD-1"), "AUD-1", opts, DispatchDeps{
		Store:           store,
		Buffer:          buf,
		IDs:             &fakeIDs{ids: []string{"AUD-2"}},
		Metrics:         &fakeRecorder{},
		IsAlreadyExists: existsPred,
	})
	if got.Buffered || got.BufferAddErr == nil || store.calls != 1 || got.Persist.Err != nil {
		t.Fatalf("%+v calls=%d", got, store.calls)
	}
}

func TestDispatchRetryClonesID(t *testing.T) {
	t.Parallel()
	store := &fakeStore{errs: []error{errExists}}
	opts := &EventOptions{EventType: EventTypeCacheInvalidation, Operation: "test", Severity: SeverityLow}
	got := Dispatch(context.Background(), nil, testBuilder(t, "AUD-1"), "AUD-1", opts, DispatchDeps{
		Store:           store,
		IDs:             &fakeIDs{ids: []string{"AUD-2"}},
		Metrics:         &fakeRecorder{},
		IsAlreadyExists: existsPred,
	})
	if got.Persist.Err != nil || !got.Persist.Retried || store.calls != 2 {
		t.Fatalf("%+v calls=%d", got.Persist, store.calls)
	}
	if objects.GetString(got.Persist.Instance, FieldID) != "AUD-2" {
		t.Fatalf("retry id=%v", got.Persist.Instance[FieldID])
	}
}

func TestDispatchPersistsWhenNotAggregated(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	opts := &EventOptions{EventType: EventTypeCacheInvalidation, Operation: "test", Severity: SeverityLow}
	got := Dispatch(context.Background(), nil, testBuilder(t, "AUD-1"), "AUD-1", opts, DispatchDeps{
		Store:   store,
		Buffer:  &fakeBuffer{aggregate: false},
		Metrics: &fakeRecorder{},
	})
	if got.Buffered || store.calls != 1 || got.Instance == nil {
		t.Fatalf("%+v calls=%d", got, store.calls)
	}
	if objects.GetString(got.Instance, objects.FieldKeyID) != "AUD-1" {
		t.Fatalf("id=%v", got.Instance[objects.FieldKeyID])
	}
}

func TestClassifyDispatch(t *testing.T) {
	t.Parallel()
	if ClassifyDispatch(DispatchResult{BufferBuildErr: errors.New("b")}) != DispatchBufferBuildFailed {
		t.Fatal("buffer build")
	}
	if ClassifyDispatch(DispatchResult{ImmediateBuildErr: errors.New("i")}) != DispatchImmediateBuildFailed {
		t.Fatal("immediate build")
	}
	if ClassifyDispatch(DispatchResult{Buffered: true}) != DispatchBuffered {
		t.Fatal("buffered")
	}
	if ClassifyDispatch(DispatchResult{Persist: PersistResult{Retried: true}}) != DispatchRetrySucceeded {
		t.Fatal("retry")
	}
	if ClassifyDispatch(DispatchResult{Persist: PersistResult{AlreadyExists: true, Err: errors.New("dup")}}) != DispatchPersisted {
		t.Fatal("dup")
	}
	if ClassifyDispatch(DispatchResult{Persist: PersistResult{Err: errors.New("disk")}}) != DispatchFailed {
		t.Fatal("fail")
	}
}

func TestCacheableEventIDAndPersistLog(t *testing.T) {
	t.Parallel()
	ok := DispatchResult{Instance: map[string]any{FieldID: "AUD-1"}}
	if CacheableEventID(ok) != "AUD-1" || ClassifyPersistLog(ok) != PersistLogNone {
		t.Fatalf("%+v", ok)
	}
	dup := DispatchResult{
		Instance: map[string]any{FieldID: "AUD-1"},
		Persist:  PersistResult{Err: errors.New("dup"), AlreadyExists: true},
	}
	if CacheableEventID(dup) != "AUD-1" || ClassifyPersistLog(dup) != PersistLogDuplicate {
		t.Fatalf("%+v", dup)
	}
	fail := DispatchResult{Persist: PersistResult{Err: errors.New("disk")}}
	if CacheableEventID(fail) != "" || ClassifyPersistLog(fail) != PersistLogError {
		t.Fatalf("%+v", fail)
	}
	if CacheableEventID(DispatchResult{Buffered: true, Instance: map[string]any{FieldID: "AUD-1"}}) != "" {
		t.Fatal("buffered should not cache")
	}
}
