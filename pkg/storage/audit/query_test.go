package audit

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

type fakeQuery struct {
	last ListQuery
	objs []map[string]any
	err  error
}

func (f *fakeQuery) List(_ context.Context, _ *pkgctx.SecurityContext, _ *pkgctx.StorageContext, q ListQuery) ([]map[string]any, error) {
	f.last = q
	return f.objs, f.err
}

var _ ObjectQuery = (*fakeQuery)(nil)

func TestEventsOldestFirst(t *testing.T) {
	t.Parallel()
	q := EventsOldestFirst("audit_event", StatusEq("archived"), 0)
	if q.Kind != "audit_event" || q.SortBy != objects.FieldKeyCreatedAt || !q.SortAsc || q.Limit != 0 {
		t.Fatalf("%+v", q)
	}
}

func TestEventsNewestFirstAndFirstMatch(t *testing.T) {
	t.Parallel()
	newest := EventsNewestFirst("audit_aggregation_metric", SourceEq("job"), 10)
	if newest.Kind != "audit_aggregation_metric" || newest.SortAsc || newest.Limit != 10 {
		t.Fatalf("%+v", newest)
	}
	first := FirstMatch("audit_aggregation_metric", ExactAggregationWindow("a", "b"))
	if first.Limit != 1 || first.SortBy != "" {
		t.Fatalf("%+v", first)
	}
}

func TestLimitedAndFirstObject(t *testing.T) {
	t.Parallel()
	q := Limited("audit_event", IDsIn([]string{"AUD-1"}), 3)
	if q.Limit != 3 || q.SortBy != "" {
		t.Fatalf("%+v", q)
	}
	if FirstObject(nil) != nil || FirstObject([]map[string]any{}) != nil {
		t.Fatal("empty should be nil")
	}
	got := FirstObject([]map[string]any{{objects.FieldKeyID: "AUD-1"}})
	if objects.GetString(got, objects.FieldKeyID) != "AUD-1" {
		t.Fatalf("%v", got)
	}
}

func TestObjectQueryReceivesListQuery(t *testing.T) {
	t.Parallel()
	fq := &fakeQuery{objs: []map[string]any{{objects.FieldKeyID: "AUD-9"}}}
	cutoff := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	q := EventsOldestFirst("audit_event", CreatedAtBefore(cutoff), 5)
	objs, err := fq.List(context.Background(), nil, nil, q)
	if err != nil || len(objs) != 1 || fq.last.Limit != 5 || fq.last.Kind != "audit_event" {
		t.Fatalf("objs=%v last=%+v err=%v", objs, fq.last, err)
	}
}
