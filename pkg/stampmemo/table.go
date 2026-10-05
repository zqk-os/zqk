package stampmemo

import (
	"sync"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Stamp is a filesystem generation, typically mtime nanoseconds.
// Zero means the path is missing or unreadable. Stamp is not a cache key.
type Stamp int64

// Of returns the mtime stamp for path. Missing paths are Stamp(0).
func Of(path string) Stamp {
	info, err := fileutil.Stat(path)
	if err != nil {
		return 0
	}
	return Stamp(info.ModTime().UnixNano())
}

type entry[T any] struct {
	mu     sync.Mutex
	stamp  Stamp
	loaded bool
	val    T
	err    error
}

func (e *entry[T]) load(stamp Stamp, loadFn func() (T, error)) (T, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loaded && e.stamp == stamp && e.err == nil {
		return e.val, nil
	}
	val, err := loadFn()
	if err != nil {
		return val, err
	}
	e.loaded = true
	e.stamp = stamp
	e.val = val
	e.err = nil
	return val, nil
}

// Table memos T by a stable string key. A stamp change updates that entry in
// place; it does not insert a second row. Unbounded key sets will leak — see
// the package comment.
type Table[T any] struct {
	m sync.Map
}

func (t *Table[T]) slot(key string) *entry[T] {
	if existing, ok := t.m.Load(key); ok {
		return existing.(*entry[T])
	}
	fresh := &entry[T]{}
	actual, _ := t.m.LoadOrStore(key, fresh)
	return actual.(*entry[T])
}

// Load returns the retained value while stamp is unchanged. loadFn runs on miss
// or stamp change and replaces the existing entry.
func (t *Table[T]) Load(key string, stamp Stamp, loadFn func() (T, error)) (T, error) {
	return t.slot(key).load(stamp, loadFn)
}

// Peek returns the retained value if key is already loaded at this stamp.
// It never runs loadFn. If a Load is in progress it returns false immediately
// so hot paths can stay non-blocking (GetFieldsForKindIfLoaded).
func (t *Table[T]) Peek(key string, stamp Stamp) (T, bool) {
	var zero T
	v, ok := t.m.Load(key)
	if !ok {
		return zero, false
	}
	e := v.(*entry[T])
	if !e.mu.TryLock() {
		return zero, false
	}
	defer e.mu.Unlock()
	if !e.loaded || e.stamp != stamp || e.err != nil {
		return zero, false
	}
	return e.val, true
}

// Delete drops the memo for key after an out-of-band write (mtime may not have
// moved yet). It is not required when the stamp already changed.
func (t *Table[T]) Delete(key string) {
	t.m.Delete(key)
}

// Reset drops every memo because the key space itself is invalid (chdir in
// tests, explicit ClearCache). It is not a size cap or idle timeout.
func (t *Table[T]) Reset() {
	t.m.Range(func(k, _ any) bool {
		t.m.Delete(k)
		return true
	})
}
