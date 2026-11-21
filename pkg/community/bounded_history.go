package community

import "sync"

type boundedHistory[T any] struct {
	mu        sync.RWMutex
	items     []T
	maxEvents int
}

func newBoundedHistory[T any](maxHistory int) boundedHistory[T] {
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	return boundedHistory[T]{
		items:     make([]T, 0, maxHistory),
		maxEvents: maxHistory,
	}
}

func (h *boundedHistory[T]) record(item T) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.items) >= h.maxEvents {
		h.items = h.items[1:]
	}
	h.items = append(h.items, item)
}

func (h *boundedHistory[T]) snapshot() []T {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]T, len(h.items))
	copy(result, h.items)
	return result
}
