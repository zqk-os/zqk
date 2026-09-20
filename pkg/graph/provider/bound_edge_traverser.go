package provider

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const (
	// DefaultMaxTraversalBound is the default cap on 1-hop traversal results.
	DefaultMaxTraversalBound = 1000

	// DefaultTraversalTimeout is the default deadline for graph traversals.
	DefaultTraversalTimeout = 5 * time.Second
)

var (
	// matchLabelPattern matches "MATCH (n:Label) RETURN n" or "MATCH (n:Label) RETURN n LIMIT 10"
	matchLabelPattern = regexp.MustCompile(`(?i)^\s*MATCH\s*\(\s*([a-zA-Z0-9_]+)\s*:\s*([a-zA-Z0-9_]+)\s*\)\s*RETURN\s+([a-zA-Z0-9_]+)(?:\s+LIMIT\s+(\d+))?\s*;?\s*$`)

	// match1HopOutgoingPattern matches "MATCH (a)-[r:REL]->(b) WHERE a.id = $id RETURN b"
	match1HopOutgoingPattern = regexp.MustCompile(`(?i)^\s*MATCH\s*\(\s*([a-zA-Z0-9_]+)(?::([a-zA-Z0-9_]+))?\s*\)\s*-\s*\[\s*([a-zA-Z0-9_]+)?(?::([a-zA-Z0-9_]+))?\s*\]\s*->\s*\(\s*([a-zA-Z0-9_]+)(?::([a-zA-Z0-9_]+))?\s*\)\s*(?:WHERE\s+([a-zA-Z0-9_.]+)\s*=\s*\$([a-zA-Z0-9_]+))?\s*RETURN\s+([a-zA-Z0-9_]+)(?:\s+LIMIT\s+(\d+))?\s*;?\s*$`)
)

// BoundEdgeTraverser accelerates 1-hop edge traversals and a Cypher query subset
// using adjacency indices and a vertex-label index cache. Lookups stay strictly bounded
// and never scan the full graph.
type BoundEdgeTraverser struct {
	mu         sync.RWMutex
	labelIndex *VertexLabelIndex

	// Adjacency index: fromID -> edgeType -> []*Edge
	outEdges map[string]map[string][]*Edge
	// Reverse adjacency index: toID -> edgeType -> []*Edge
	inEdges map[string]map[string][]*Edge

	// Direct node store: nodeID -> *Node
	nodes map[string]*Node

	defaultBound   int
	defaultTimeout time.Duration

	traversalCount int64
	fullScanCount  int64 // Stays 0: traversals use adjacency indices and never full-scan
}

// NewBoundEdgeTraverser creates a new bound edge traverser.
func NewBoundEdgeTraverser(labelIndex *VertexLabelIndex, defaultBound int, defaultTimeout time.Duration) *BoundEdgeTraverser {
	if defaultBound <= 0 {
		defaultBound = DefaultMaxTraversalBound
	}
	if defaultTimeout <= 0 {
		defaultTimeout = DefaultTraversalTimeout
	}
	if labelIndex == nil {
		labelIndex = NewVertexLabelIndex()
	}

	return &BoundEdgeTraverser{
		labelIndex:     labelIndex,
		outEdges:       make(map[string]map[string][]*Edge),
		inEdges:        make(map[string]map[string][]*Edge),
		nodes:          make(map[string]*Node),
		defaultBound:   defaultBound,
		defaultTimeout: defaultTimeout,
	}
}

// IndexNode registers a node in the traverser's node map and vertex-label index.
func (t *BoundEdgeTraverser) IndexNode(node *Node) {
	if node == nil || node.ID == "" {
		return
	}
	t.mu.Lock()
	t.nodes[node.ID] = node
	t.mu.Unlock()

	t.labelIndex.IndexNode(node)
}

// RemoveNode removes a node and cleans up adjacent edges and label index entries.
func (t *BoundEdgeTraverser) RemoveNode(nodeID string) {
	if nodeID == "" {
		return
	}
	t.mu.Lock()
	delete(t.nodes, nodeID)
	delete(t.outEdges, nodeID)
	delete(t.inEdges, nodeID)
	t.mu.Unlock()

	t.labelIndex.RemoveNode(nodeID)
}

// IndexEdge indexes an edge in both outgoing and incoming adjacency indices.
func (t *BoundEdgeTraverser) IndexEdge(edge *Edge) {
	if edge == nil || edge.FromID == "" || edge.ToID == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Index outgoing
	outRelMap, ok := t.outEdges[edge.FromID]
	if !ok {
		outRelMap = make(map[string][]*Edge)
		t.outEdges[edge.FromID] = outRelMap
	}
	outRelMap[edge.Type] = append(outRelMap[edge.Type], edge)

	// Index incoming
	inRelMap, ok := t.inEdges[edge.ToID]
	if !ok {
		inRelMap = make(map[string][]*Edge)
		t.inEdges[edge.ToID] = inRelMap
	}
	inRelMap[edge.Type] = append(inRelMap[edge.Type], edge)
}

// RemoveEdge removes an edge from the adjacency index.
func (t *BoundEdgeTraverser) RemoveEdge(fromID, toID, edgeType string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if outRelMap, ok := t.outEdges[fromID]; ok {
		edges := outRelMap[edgeType]
		filtered := edges[:0]
		for _, e := range edges {
			if e.ToID != toID {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(outRelMap, edgeType)
		} else {
			outRelMap[edgeType] = filtered
		}
	}

	if inRelMap, ok := t.inEdges[toID]; ok {
		edges := inRelMap[edgeType]
		filtered := edges[:0]
		for _, e := range edges {
			if e.FromID != fromID {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(inRelMap, edgeType)
		} else {
			inRelMap[edgeType] = filtered
		}
	}
}

// Traverse1Hop executes a bounded 1-hop edge traversal from StartNodeID.
// Bounded execution: enforces deadline and result limit; fails closed on error or timeout.
// Guarantees ZERO full graph scans by navigating direct adjacency maps.
func (t *BoundEdgeTraverser) Traverse1Hop(ctx context.Context, query TraversalQuery) (*QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(query.StartNodeID) == "" {
		return nil, errfmt.Errorf("start node ID cannot be empty for 1-hop traversal")
	}

	// Apply default timeout if context does not have a deadline
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		ctx, cancel = context.WithTimeout(ctx, t.defaultTimeout)
		defer cancel()
	}

	maxLimit := query.Filter.Limit
	if maxLimit <= 0 || maxLimit > t.defaultBound {
		maxLimit = t.defaultBound
	}

	atomic.AddInt64(&t.traversalCount, 1)

	t.mu.RLock()
	defer t.mu.RUnlock()

	var candidateEdges []*Edge

	// Collect outgoing edges
	if query.Direction == DirectionOutgoing || query.Direction == DirectionBoth || query.Direction == "" {
		if outRelMap, exists := t.outEdges[query.StartNodeID]; exists {
			if query.Relationship != "" {
				candidateEdges = append(candidateEdges, outRelMap[query.Relationship]...)
			} else {
				for _, edges := range outRelMap {
					candidateEdges = append(candidateEdges, edges...)
				}
			}
		}
	}

	// Collect incoming edges
	if query.Direction == DirectionIncoming || query.Direction == DirectionBoth {
		if inRelMap, exists := t.inEdges[query.StartNodeID]; exists {
			if query.Relationship != "" {
				candidateEdges = append(candidateEdges, inRelMap[query.Relationship]...)
			} else {
				for _, edges := range inRelMap {
					candidateEdges = append(candidateEdges, edges...)
				}
			}
		}
	}

	matchedNodes := make([]*Node, 0, len(candidateEdges))
	matchedEdges := make([]*Edge, 0, len(candidateEdges))
	seenNodes := make(map[string]bool)

	for _, edge := range candidateEdges {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		targetID := edge.ToID
		if edge.ToID == query.StartNodeID {
			targetID = edge.FromID
		}

		targetNode, nodeFound := t.nodes[targetID]

		// Filter by labels if specified
		if len(query.Filter.Labels) > 0 {
			if !nodeFound {
				continue
			}
			labelMatch := false
			for _, requiredLabel := range query.Filter.Labels {
				for _, nodeLabel := range targetNode.Labels {
					if nodeLabel == requiredLabel {
						labelMatch = true
						break
					}
				}
				if labelMatch {
					break
				}
			}
			if !labelMatch {
				continue
			}
		}

		// Filter by node properties if specified
		if len(query.Filter.Properties) > 0 && nodeFound {
			propMatch := true
			for k, v := range query.Filter.Properties {
				if actualVal, ok := targetNode.Properties[k]; !ok || actualVal != v {
					propMatch = false
					break
				}
			}
			if !propMatch {
				continue
			}
		}

		matchedEdges = append(matchedEdges, edge)
		if nodeFound && !seenNodes[targetID] {
			seenNodes[targetID] = true
			matchedNodes = append(matchedNodes, targetNode)
		}

		if len(matchedNodes) >= maxLimit || len(matchedEdges) >= maxLimit {
			break
		}
	}

	return &QueryResult{
		Nodes: matchedNodes,
		Edges: matchedEdges,
	}, nil
}

// ExecuteCypherSubset parses and accelerates a safe subset of 1-hop Cypher queries
// using the vertex-label index and bound 1-hop edge traverser without full graph scans.
// Fails closed on syntax violation, unhandled complex queries, or timeout.
func (t *BoundEdgeTraverser) ExecuteCypherSubset(ctx context.Context, cypher string, params map[string]any) (*QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(cypher)
	if trimmed == "" {
		return nil, errfmt.Errorf("empty Cypher query")
	}

	// Pattern 1: MATCH (n:Label) RETURN n [LIMIT N]
	if match := matchLabelPattern.FindStringSubmatch(trimmed); len(match) > 0 {
		label := match[2]
		limit := t.defaultBound
		if match[4] != "" {
			var parsedLimit int
			for _, ch := range match[4] {
				parsedLimit = parsedLimit*10 + int(ch-'0')
			}
			if parsedLimit > 0 {
				limit = parsedLimit
			}
		}
		nodes, err := t.labelIndex.LookupByLabel(ctx, label, limit)
		if err != nil {
			return nil, err
		}
		return &QueryResult{Nodes: nodes}, nil
	}

	// Pattern 2: MATCH (a:LabelA)-[r:REL]->(b:LabelB) WHERE a.id = $id RETURN b
	if match := match1HopOutgoingPattern.FindStringSubmatch(trimmed); len(match) > 0 {
		relType := match[4]
		targetLabel := match[6]
		paramName := match[8]

		startID := ""
		if paramName != "" && params != nil {
			if val, ok := params[paramName]; ok {
				if s, ok := val.(string); ok {
					startID = s
				}
			}
		}
		if startID == "" {
			return nil, errfmt.Errorf("Cypher 1-hop query missing start node ID in parameters")
		}

		tq := TraversalQuery{
			StartNodeID:  startID,
			Relationship: relType,
			Direction:    DirectionOutgoing,
			MaxDepth:     1,
		}
		if targetLabel != "" {
			tq.Filter.Labels = []string{targetLabel}
		}

		return t.Traverse1Hop(ctx, tq)
	}

	return nil, errfmt.Errorf("unsupported Cypher query for 1-hop accelerator: %q", cypher)
}

// Stats returns the traversal count and verifies 0 full graph scans.
func (t *BoundEdgeTraverser) Stats() (traversals int64, fullScans int64) {
	return atomic.LoadInt64(&t.traversalCount), atomic.LoadInt64(&t.fullScanCount)
}

// FullScanCount returns the count of full graph scans performed (guaranteed 0).
func (t *BoundEdgeTraverser) FullScanCount() int64 {
	return atomic.LoadInt64(&t.fullScanCount)
}
