package stampmemo

import "sync"

type viewHit[T any] struct {
	raw []byte
	val T
}

// View retains a parsed projection of a byte slice until the backing array
// changes. Keys are unbounded like Table — use a closed identity, not a new
// key per payload generation.
type View[T any] struct {
	m sync.Map
}

// SameBacking reports whether a and b share the same underlying array.
func SameBacking(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}

// Get returns the parsed value for key. parse runs only when raw is a new backing array.
func (v *View[T]) Get(key string, raw []byte, parse func([]byte) (T, error)) (T, error) {
	if hit, ok := v.m.Load(key); ok {
		parsed := hit.(viewHit[T])
		if SameBacking(parsed.raw, raw) {
			return parsed.val, nil
		}
	}
	var zero T
	val, err := parse(raw)
	if err != nil {
		return zero, err
	}
	v.m.Store(key, viewHit[T]{raw: raw, val: val})
	return val, nil
}
