package graph

import (
	"context"
	"errors"

	"github.com/lanceman/zqk/pkg/ontology"
)

var (
	ErrNodeNotFound     = errors.New("node not found in graph")
	ErrMaxDepthExceeded = errors.New("maximum traversal depth exceeded")
)

type Node struct {
	ID    string
	Class ontology.Class
	Data  map[string]any
}

type Edge struct {
	SourceID string
	TargetID string
	Relation string
}

// MemoryOntologyBridge defines the interface for semantic graph traversal.
type MemoryOntologyBridge interface {
	Traverse(ctx context.Context, startNodeID string, maxDepth int) ([]Node, []Edge, error)
	GetRelated(ctx context.Context, nodeID string, relation string) ([]Node, error)
}

// GraphProvider is the abstract interface the bridge uses to talk to underlying storage.
type GraphProvider interface {
	GetNode(ctx context.Context, id string) (Node, error)
	GetOutboundEdges(ctx context.Context, sourceID string) ([]Edge, error)
}

type defaultBridge struct {
	provider GraphProvider
}

// NewBridge creates a new MemoryOntologyBridge.
func NewBridge(provider GraphProvider) MemoryOntologyBridge {
	return &defaultBridge{
		provider: provider,
	}
}

func (b *defaultBridge) Traverse(ctx context.Context, startNodeID string, maxDepth int) ([]Node, []Edge, error) {
	if maxDepth < 0 {
		return nil, nil, ErrMaxDepthExceeded
	}

	visited := make(map[string]bool)
	var nodes []Node
	var edges []Edge

	// BFS queue
	type queueItem struct {
		id    string
		depth int
	}
	queue := []queueItem{{id: startNodeID, depth: 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.id] {
			continue
		}
		visited[current.id] = true

		node, err := b.provider.GetNode(ctx, current.id)
		if err != nil {
			continue // Skip missing nodes quietly for robust traversal
		}
		nodes = append(nodes, node)

		if current.depth < maxDepth {
			outboundEdges, err := b.provider.GetOutboundEdges(ctx, current.id)
			if err == nil {
				edges = append(edges, outboundEdges...)
				for _, edge := range outboundEdges {
					if !visited[edge.TargetID] {
						queue = append(queue, queueItem{id: edge.TargetID, depth: current.depth + 1})
					}
				}
			}
		}
	}

	if len(nodes) == 0 && !visited[startNodeID] {
		return nil, nil, ErrNodeNotFound
	}

	return nodes, edges, nil
}

func (b *defaultBridge) GetRelated(ctx context.Context, nodeID string, relation string) ([]Node, error) {
	edges, err := b.provider.GetOutboundEdges(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	var nodes []Node
	for _, edge := range edges {
		if edge.Relation == relation {
			node, err := b.provider.GetNode(ctx, edge.TargetID)
			if err == nil {
				nodes = append(nodes, node)
			}
		}
	}

	return nodes, nil
}
