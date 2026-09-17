package provider

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/errfmt"
)

const (
	// DefaultMaxLookupBound is the default upper bound for vertex-label lookups.
	DefaultMaxLookupBound = 5000
)

// VertexLabelIndex provides an embedded, thread-safe inverted index from vertex labels
// to nodes. Lookups are direct index hits (O(1) map access) and never scan the full graph.
type VertexLabelIndex struct {
	mu            sync.RWMutex
	labelToNodes  map[string]map[string]*Node // label -> nodeID -> *Node
	nodeToLabels  map[string][]string         // nodeID -> []label
	indexHits     int64
	indexMisses   int64
	fullScanCount int64 // Stays 0: index lookups never perform a full graph scan
}

// NewVertexLabelIndex initializes an empty vertex-label index.
func NewVertexLabelIndex() *VertexLabelIndex {
	return &VertexLabelIndex{
		labelToNodes: make(map[string]map[string]*Node),
		nodeToLabels: make(map[string][]string),
	}
}

// IndexNode indexes or updates a node under its declared labels.
// Thread-safe: acquires write lock.
func (idx *VertexLabelIndex) IndexNode(node *Node) {
	if node == nil || node.ID == "" {
		return
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Clear previous label mappings if the node was already indexed
	if oldLabels, exists := idx.nodeToLabels[node.ID]; exists {
		for _, oldLabel := range oldLabels {
			if nodeMap, ok := idx.labelToNodes[oldLabel]; ok {
				delete(nodeMap, node.ID)
				if len(nodeMap) == 0 {
					delete(idx.labelToNodes, oldLabel)
				}
			}
		}
	}

	// Index under new labels
	labels := make([]string, len(node.Labels))
	copy(labels, node.Labels)
	idx.nodeToLabels[node.ID] = labels

	for _, label := range labels {
		if label == "" {
			continue
		}
		nodeMap, ok := idx.labelToNodes[label]
		if !ok {
			nodeMap = make(map[string]*Node)
			idx.labelToNodes[label] = nodeMap
		}
		nodeMap[node.ID] = node
	}
}

// RemoveNode removes a node and all its label associations from the index.
func (idx *VertexLabelIndex) RemoveNode(nodeID string) {
	if nodeID == "" {
		return
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	oldLabels, exists := idx.nodeToLabels[nodeID]
	if !exists {
		return
	}

	for _, label := range oldLabels {
		if nodeMap, ok := idx.labelToNodes[label]; ok {
			delete(nodeMap, nodeID)
			if len(nodeMap) == 0 {
				delete(idx.labelToNodes, label)
			}
		}
	}
	delete(idx.nodeToLabels, nodeID)
}

// LookupByLabel retrieves nodes possessing the given label up to bound limit.
// Lookup is O(1) map access into the inverted index; it NEVER scans the full graph.
// Fail-closed: returns error if ctx is cancelled or label is empty.
func (idx *VertexLabelIndex) LookupByLabel(ctx context.Context, label string, bound int) ([]*Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if label == "" {
		return nil, errfmt.Errorf("empty label for vertex-label index lookup")
	}

	maxBound := bound
	if maxBound <= 0 || maxBound > DefaultMaxLookupBound {
		maxBound = DefaultMaxLookupBound
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	nodeMap, found := idx.labelToNodes[label]
	if !found || len(nodeMap) == 0 {
		atomic.AddInt64(&idx.indexMisses, 1)
		return nil, nil
	}

	atomic.AddInt64(&idx.indexHits, 1)

	count := len(nodeMap)
	if count > maxBound {
		count = maxBound
	}

	results := make([]*Node, 0, count)
	for _, node := range nodeMap {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		results = append(results, node)
		if len(results) >= maxBound {
			break
		}
	}

	return results, nil
}

// NodeCountForLabel returns the number of nodes indexed under a label.
func (idx *VertexLabelIndex) NodeCountForLabel(label string) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.labelToNodes[label])
}

// TotalIndexedNodes returns the total count of distinct nodes indexed.
func (idx *VertexLabelIndex) TotalIndexedNodes() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.nodeToLabels)
}

// IndexedLabels returns a slice of all currently indexed labels.
func (idx *VertexLabelIndex) IndexedLabels() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	labels := make([]string, 0, len(idx.labelToNodes))
	for l := range idx.labelToNodes {
		labels = append(labels, l)
	}
	return labels
}

// Stats returns index performance and hit statistics.
func (idx *VertexLabelIndex) Stats() (hits int64, misses int64, fullScans int64) {
	return atomic.LoadInt64(&idx.indexHits), atomic.LoadInt64(&idx.indexMisses), atomic.LoadInt64(&idx.fullScanCount)
}

// FullScanCount returns the count of full graph scans performed.
// Guaranteed to remain 0 when using the index.
func (idx *VertexLabelIndex) FullScanCount() int64 {
	return atomic.LoadInt64(&idx.fullScanCount)
}
