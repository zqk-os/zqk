package compose

import "sync"

// Registry is an in-memory map of CompositionKey → Definition.
type Registry struct {
	mu    sync.RWMutex
	byKey map[string]*Definition
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byKey: make(map[string]*Definition)}
}

// Put stores or replaces a definition.
func (r *Registry) Put(d *Definition) {
	if r == nil || d == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byKey[d.Key.String()] = d
}

// Get looks up by key.
func (r *Registry) Get(k CompositionKey) (*Definition, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byKey[k.String()]
	return d, ok
}

// GetByParts looks up by object kind, pipeline kind, and intent.
func (r *Registry) GetByParts(objectKind, pipelineKind, intent string) (*Definition, bool) {
	return r.Get(CompositionKey{ObjectKind: objectKind, PipelineKind: pipelineKind, Intent: intent})
}

// Len returns the number of definitions.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byKey)
}

// All returns a snapshot of all definitions.
func (r *Registry) All() []*Definition {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Definition, 0, len(r.byKey))
	for _, d := range r.byKey {
		out = append(out, d)
	}
	return out
}

// CoverageCount returns how many definitions exist for objectKind (any intent).
func (r *Registry) CoverageCount(objectKind string) int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, d := range r.byKey {
		if d.Key.ObjectKind == objectKind {
			n++
		}
	}
	return n
}
