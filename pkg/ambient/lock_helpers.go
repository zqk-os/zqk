package ambient

import "sync"

func withRLock[T any](mu *sync.RWMutex, fn func() T) T {
	mu.RLock()
	defer mu.RUnlock()
	return fn()
}

func withWLock(mu *sync.RWMutex, fn func()) {
	mu.Lock()
	defer mu.Unlock()
	fn()
}
