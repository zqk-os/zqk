package datacell

import (
	"sync"
	"sync/atomic"
)

type StewardshipRegistry struct {
	mu     sync.Mutex
	counts atomic.Pointer[map[string]int]
}

func NewStewardshipRegistry() *StewardshipRegistry {
	r := &StewardshipRegistry{}
	initial := make(map[string]int)
	r.counts.Store(&initial)
	return r
}

func (r *StewardshipRegistry) UpdateCount(kind string, count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := *r.counts.Load()
	next := make(map[string]int, len(current)+1)
	for k, v := range current {
		next[k] = v
	}
	next[kind] = count
	r.counts.Store(&next)
}

func (r *StewardshipRegistry) GetCount(kind string) (int, bool) {
	current := *r.counts.Load()
	val, ok := current[kind]
	return val, ok
}

func (r *StewardshipRegistry) GetAllCounts() map[string]int {
	current := *r.counts.Load()
	out := make(map[string]int, len(current))
	for k, v := range current {
		out[k] = v
	}
	return out
}

var globalRegistry = NewStewardshipRegistry()

func GetGlobalStewardshipRegistry() *StewardshipRegistry {
	return globalRegistry
}
