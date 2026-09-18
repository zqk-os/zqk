package audit

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

type fakeBulk struct {
	ids    []string
	status string
	n      int
	err    error
}

func (f *fakeBulk) UpdateStatus(_ context.Context, _ *pkgctx.SecurityContext, ids []string, status string) (int, error) {
	f.ids = ids
	f.status = status
	if f.err != nil {
		return 0, f.err
	}
	return f.n, nil
}

var _ BulkStatusWriter = (*fakeBulk)(nil)

func TestBulkStatusWriterRecords(t *testing.T) {
	t.Parallel()
	w := &fakeBulk{n: 2}
	got, err := w.UpdateStatus(context.Background(), nil, []string{"AUD-1", "AUD-2"}, "archived")
	if err != nil || got != 2 || w.status != "archived" || len(w.ids) != 2 {
		t.Fatalf("got=%d err=%v w=%+v", got, err, w)
	}
}

func TestBulkStatusWriterError(t *testing.T) {
	t.Parallel()
	want := errors.New("bulk")
	w := &fakeBulk{err: want}
	_, err := w.UpdateStatus(context.Background(), nil, []string{"AUD-1"}, "archived")
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}

func TestBulkDeleteOutcomeAllFailed(t *testing.T) {
	t.Parallel()
	if (BulkDeleteOutcome{}).AllFailed() {
		t.Fatal("empty should not be all-failed")
	}
	if !(BulkDeleteOutcome{FailureCount: 3}).AllFailed() {
		t.Fatal("failures with zero success")
	}
	if (BulkDeleteOutcome{SuccessCount: 1, FailureCount: 2}).AllFailed() {
		t.Fatal("partial success is not all-failed")
	}
	if (BulkDeleteOutcome{FailureCount: 3, Optimized: true}).WrapAllFailed() {
		t.Fatal("optimized path does not wrap")
	}
	if !(BulkDeleteOutcome{FailureCount: 3}).WrapAllFailed() {
		t.Fatal("generic path wraps all-failed")
	}
}
