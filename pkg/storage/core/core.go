package core

// Engine defines the core interface required by all storage subsystems.
type Engine interface {
	Kind() string
	Close() error
}

// MemoryEngine is an in-memory implementation of the core storage engine.
type MemoryEngine struct {
	kind string
}

// NewMemoryEngine creates a new core in-memory engine.
func NewMemoryEngine(kind string) *MemoryEngine {
	return &MemoryEngine{kind: kind}
}

func (m *MemoryEngine) Kind() string {
	return m.kind
}

func (m *MemoryEngine) Close() error {
	return nil
}
