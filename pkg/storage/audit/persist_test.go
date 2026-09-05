package audit

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

var errExists = errors.New("already exists")

type fakeStore struct {
	calls int
	errs  []error
}

func (f *fakeStore) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	i := f.calls
	f.calls++
	if i < len(f.errs) {
		return f.errs[i]
	}
	return nil
}

func existsPred(err error) bool {
	return IsAlreadyExists(err, errExists, "already exists")
}

func TestPersistWithIDRetrySuccess(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	first := map[string]any{"id": "AUD-1"}
	got := PersistWithIDRetry(context.Background(), store, nil, first, existsPred, func() (map[string]any, error) {
		t.Fatal("retry should not run")
		return nil, nil
	})
	if got.Err != nil || got.AlreadyExists || got.Retried || store.calls != 1 {
		t.Fatalf("%+v calls=%d", got, store.calls)
	}
}

func TestPersistWithIDRetryDuplicateThenOK(t *testing.T) {
	t.Parallel()
	store := &fakeStore{errs: []error{errExists}}
	first := map[string]any{"id": "AUD-1"}
	second := map[string]any{"id": "AUD-2"}
	got := PersistWithIDRetry(context.Background(), store, nil, first, existsPred, func() (map[string]any, error) {
		return second, nil
	})
	if got.Err != nil || got.AlreadyExists || !got.Retried {
		t.Fatalf("%+v", got)
	}
	if got.Instance["id"] != "AUD-2" || store.calls != 2 {
		t.Fatalf("instance=%v calls=%d", got.Instance, store.calls)
	}
}

func TestPersistWithIDRetryDuplicateRetryAlsoExists(t *testing.T) {
	t.Parallel()
	store := &fakeStore{errs: []error{errExists, errExists}}
	got := PersistWithIDRetry(context.Background(), store, nil, map[string]any{"id": "AUD-1"}, existsPred, func() (map[string]any, error) {
		return map[string]any{"id": "AUD-2"}, nil
	})
	if !got.AlreadyExists || !got.Retried || got.Err == nil {
		t.Fatalf("%+v", got)
	}
}

func TestPersistWithIDRetryOtherErrorSkipsRetry(t *testing.T) {
	t.Parallel()
	other := errors.New("validation failed")
	store := &fakeStore{errs: []error{other}}
	got := PersistWithIDRetry(context.Background(), store, nil, map[string]any{"id": "AUD-1"}, existsPred, func() (map[string]any, error) {
		t.Fatal("retry should not run")
		return nil, nil
	})
	if got.AlreadyExists || got.Retried || !errors.Is(got.Err, other) || store.calls != 1 {
		t.Fatalf("%+v calls=%d", got, store.calls)
	}
}

func TestPersistWithIDRetryNilRetryKeepsOriginal(t *testing.T) {
	t.Parallel()
	store := &fakeStore{errs: []error{errExists}}
	first := map[string]any{"id": "AUD-1"}
	got := PersistWithIDRetry(context.Background(), store, nil, first, existsPred, func() (map[string]any, error) {
		return nil, nil
	})
	if !got.AlreadyExists || got.Retried || store.calls != 1 {
		t.Fatalf("%+v calls=%d", got, store.calls)
	}
}
