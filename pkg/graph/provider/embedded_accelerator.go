package provider

import (
	"context"
	"time"
)

// EmbeddedGraphAccelerator provides a high-performance in-memory index cache and 1-hop
// traversal accelerator for embedded graph operations. Lookups and traversals are strictly
// bounded in time and size and operate in O(1) or O(degree) without full graph scans.
type EmbeddedGraphAccelerator struct {
	labelIndex *VertexLabelIndex
	traverser  *BoundEdgeTraverser
}

// NewEmbeddedGraphAccelerator initializes a graph accelerator with bounded limits and timeouts.
func NewEmbeddedGraphAccelerator(defaultBound int, defaultTimeout time.Duration) *EmbeddedGraphAccelerator {
	labelIdx := NewVertexLabelIndex()
	traverser := NewBoundEdgeTraverser(labelIdx, defaultBound, defaultTimeout)
	return &EmbeddedGraphAccelerator{
		labelIndex: labelIdx,
		traverser:  traverser,
	}
}

// LabelIndex returns the underlying vertex-label inverted index.
func (a *EmbeddedGraphAccelerator) LabelIndex() *VertexLabelIndex {
	return a.labelIndex
}

// Traverser returns the underlying bound edge traverser.
func (a *EmbeddedGraphAccelerator) Traverser() *BoundEdgeTraverser {
	return a.traverser
}

// IndexNode indexes a node in both vertex-label cache and traversal node map.
func (a *EmbeddedGraphAccelerator) IndexNode(node *Node) {
	a.traverser.IndexNode(node)
}

// RemoveNode unregisters a node from the accelerator.
func (a *EmbeddedGraphAccelerator) RemoveNode(nodeID string) {
	a.traverser.RemoveNode(nodeID)
}

// IndexEdge indexes an edge in the adjacency maps for sub-millisecond 1-hop traversals.
func (a *EmbeddedGraphAccelerator) IndexEdge(edge *Edge) {
	a.traverser.IndexEdge(edge)
}

// RemoveEdge removes an edge from adjacency indexing.
func (a *EmbeddedGraphAccelerator) RemoveEdge(fromID, toID, edgeType string) {
	a.traverser.RemoveEdge(fromID, toID, edgeType)
}

// LookupByLabel performs an O(1) vertex-label index lookup without a full graph scan.
// Bounded and fail-closed on context cancellation.
func (a *EmbeddedGraphAccelerator) LookupByLabel(ctx context.Context, label string, bound int) ([]*Node, error) {
	return a.labelIndex.LookupByLabel(ctx, label, bound)
}

// Traverse1Hop executes a bounded 1-hop traversal via adjacency indexing without scanning all edges.
// Bounded and fail-closed on timeout.
func (a *EmbeddedGraphAccelerator) Traverse1Hop(ctx context.Context, query TraversalQuery) (*QueryResult, error) {
	return a.traverser.Traverse1Hop(ctx, query)
}

// ExecuteCypher executes a 1-hop accelerated Cypher query without full graph scans.
func (a *EmbeddedGraphAccelerator) ExecuteCypher(ctx context.Context, cypher string, params map[string]any) (*QueryResult, error) {
	return a.traverser.ExecuteCypherSubset(ctx, cypher, params)
}

// FullScanCount returns the count of full graph scans performed.
// Guaranteed to be 0 for all accelerated operations.
func (a *EmbeddedGraphAccelerator) FullScanCount() int64 {
	return a.labelIndex.FullScanCount() + a.traverser.FullScanCount()
}
