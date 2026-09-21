package stampmemo

import (
	"sync"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Stamp is a filesystem generation, typically mtime nanoseconds.
// Zero means the path is missing or unreadable.
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
	if e.loaded && e.stamp == stamp {
		return e.val, e.err
	}
	val, err := loadFn()
	e.loaded = true
	e.stamp = stamp
	e.val = val
	e.err = err
	return val, err
}

// Table is a stamp-invalidated memo keyed by an arbitrary string (usually projectRoot or path).
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

// Load returns the retained value while stamp is unchanged. loadFn runs on miss or stamp change.
func (t *Table[T]) Load(key string, stamp Stamp, loadFn func() (T, error)) (T, error) {
	return t.slot(key).load(stamp, loadFn)
}

// Delete drops the memo for key (after an out-of-band write).
func (t *Table[T]) Delete(key string) {
	t.m.Delete(key)
}
