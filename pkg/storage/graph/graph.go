package graph

import (
	"context"
	"sync"
)

// Node represents a vertex in the graph store.
type Node struct {
	ID         string
	Kind       string
	Properties map[string]any
}

// Store provides an in-memory graph node index.
type Store struct {
	mu    sync.RWMutex
	nodes map[string]*Node
}

// NewStore initializes a graph store.
func NewStore() *Store {
	return &Store{
		nodes: make(map[string]*Node),
	}
}

// PutNode adds or updates a node.
func (s *Store) PutNode(ctx context.Context, node *Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.ID] = node
	return nil
}

// GetNode retrieves a node by ID.
func (s *Store) GetNode(ctx context.Context, id string) (*Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	return n, ok
}
