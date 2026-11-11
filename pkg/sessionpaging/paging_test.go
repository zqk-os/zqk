package sessionpaging

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRefuseStudioModulePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path    string
		wantErr bool
	}{
		{"github.com/zqk-os/zqk/pkg/sessionpaging", false},
		{"github.com/zqk-os/zqke/pkg/virtualcontext", true},
		{"example.com/studio/paging/hot", true},
		{"example.com/foo/virtualcontext/bar", true},
		{"", false},
	}
	for _, tc := range cases {
		err := RefuseStudioModulePath(tc.path)
		if tc.wantErr && err == nil {
			t.Fatalf("path %q: expected error", tc.path)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("path %q: unexpected error: %v", tc.path, err)
		}
		if tc.wantErr && !errors.Is(err, ErrStudioPagingDep) {
			t.Fatalf("path %q: want ErrStudioPagingDep, got %v", tc.path, err)
		}
	}
}

func TestStore_PutGetEvict(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.WithMaxHotPages(2)

	session := "ZQK-test-session"
	if err := store.Put(session, "hot-a", []byte("alpha")); err != nil {
		t.Fatal(err)
	}
	// Distinct mtimes so eviction order is deterministic on fast FS.
	time.Sleep(15 * time.Millisecond)
	if err := store.Put(session, "hot-b", []byte("beta")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond)
	if err := store.Put(session, "hot-c", []byte("gamma")); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get(session, "hot-a"); !errors.Is(err, ErrPageNotFound) {
		t.Fatalf("expected hot-a evicted, got err=%v", err)
	}
	got, err := store.Get(session, "hot-c")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "gamma" {
		t.Fatalf("got %q", got)
	}
	ids, err := store.ListPageIDs(session)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("want 2 pages after eviction, got %v", ids)
	}
}

func TestStore_FailClosedEmptyAndUnsafe(t *testing.T) {
	if _, err := NewStore(""); !errors.Is(err, ErrEmptyProjectRoot) {
		t.Fatalf("empty root: %v", err)
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("", "p", nil); !errors.Is(err, ErrEmptySessionID) {
		t.Fatalf("empty session: %v", err)
	}
	if err := store.Put("s", "../x", nil); !errors.Is(err, ErrInvalidPageID) {
		t.Fatalf("unsafe page: %v", err)
	}
	if _, err := store.Get("s", "missing"); !errors.Is(err, ErrPageNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestStore_PagesUnderStateTree(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("sess-1", "page-1", []byte("x")); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".zqk", "state", PagesSubdir, "*", "*", "page-1.page"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one page file under .zqk/state/%s, got %v", PagesSubdir, matches)
	}
	if _, err := os.Stat(matches[0]); err != nil {
		t.Fatal(err)
	}
}

func TestStore_ManyPagesStayBucketed(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.WithMaxHotPages(64)
	session := "sess-bucket"
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("p-%02d", i)
		if err := store.Put(session, id, []byte(id)); err != nil {
			t.Fatal(err)
		}
	}
	sessionRoot := filepath.Join(root, ".zqk", "state", PagesSubdir, session)
	ents, err := os.ReadDir(sessionRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) > 100 {
		t.Fatalf("session dir has %d top-level entries (budget ≤100)", len(ents))
	}
}
