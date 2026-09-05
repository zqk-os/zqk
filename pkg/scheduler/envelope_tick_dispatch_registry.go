package scheduler

import (
	"sync"
)

// DispatchPolicyOverride defines custom allowlist/deny-list per token or job type.
type DispatchPolicyOverride struct {
	AllowList            map[string]struct{}
	DenyList             map[string]struct{}
	BypassExecutionDepth bool
}

// PerKindOverride defines envelope parameters for specific object kinds.
type PerKindOverride struct {
	MaxTriggers int
	MaxDepth    int
	AllowList   map[string]struct{}
	DenyList    map[string]struct{}
}

// PolicyRegistry manages override policies.
type PolicyRegistry struct {
	mu             sync.RWMutex
	tokenOverrides map[string]DispatchPolicyOverride
	kindOverrides  map[string]PerKindOverride
}

var globalPolicyRegistry = &PolicyRegistry{
	tokenOverrides: make(map[string]DispatchPolicyOverride),
	kindOverrides:  make(map[string]PerKindOverride),
}

func (r *PolicyRegistry) SetOverride(key string, policy DispatchPolicyOverride) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokenOverrides[key] = policy
}

func (r *PolicyRegistry) GetOverride(key string) (DispatchPolicyOverride, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.tokenOverrides[key]
	return p, ok
}

func (r *PolicyRegistry) SetKindOverride(kind string, policy PerKindOverride) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kindOverrides[kind] = policy
}

func (r *PolicyRegistry) GetKindOverride(kind string) (PerKindOverride, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.kindOverrides[kind]
	return p, ok
}
