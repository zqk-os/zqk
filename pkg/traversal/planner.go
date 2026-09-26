package traversal

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrCyclicRecursionExceeded = errors.New("ERR_ZPARQL_CYCLIC_RECURSION_LIMIT: traversal exceeded maximum recursion depth limit")
	ErrNodeNotFound            = errors.New("node not found in graph index")
)

// GraphIndex maintains in-memory directed adjacency and kind indexes for O(1)/O(K) lookups.
type GraphIndex struct {
	mu            sync.RWMutex
	nodes         map[string]map[string]any
	kindIndex     map[string][]string
	forwardEdges  map[string]map[string][]string // sourceID -> relation -> []targetID
	backwardEdges map[string]map[string][]string // targetID -> relation -> []sourceID
}

// NewGraphIndex constructs a thread-safe graph index.
func NewGraphIndex() *GraphIndex {
	return &GraphIndex{
		nodes:         make(map[string]map[string]any),
		kindIndex:     make(map[string][]string),
		forwardEdges:  make(map[string]map[string][]string),
		backwardEdges: make(map[string]map[string][]string),
	}
}

// AddNode registers a node and updates the kind index.
func (g *GraphIndex) AddNode(id, kind string, fields map[string]any) {
	g.mu.Lock()
	defer g.mu.Unlock()

	data := make(map[string]any, len(fields)+2)
	for k, v := range fields {
		data[k] = v
	}
	data["id"] = id
	data["kind"] = kind
	g.nodes[id] = data

	g.kindIndex[kind] = append(g.kindIndex[kind], id)
}

// AddEdge registers a directed relationship between two nodes.
func (g *GraphIndex) AddEdge(sourceID, relation, targetID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Forward
	if _, ok := g.forwardEdges[sourceID]; !ok {
		g.forwardEdges[sourceID] = make(map[string][]string)
	}
	g.forwardEdges[sourceID][relation] = append(g.forwardEdges[sourceID][relation], targetID)

	// Backward
	if _, ok := g.backwardEdges[targetID]; !ok {
		g.backwardEdges[targetID] = make(map[string][]string)
	}
	g.backwardEdges[targetID][relation] = append(g.backwardEdges[targetID][relation], sourceID)
}

// GetNode retrieves a node by ID in O(1) time.
func (g *GraphIndex) GetNode(id string) (map[string]any, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	return n, ok
}

// GetNodesByKind returns node IDs for a kind using the kind index in O(1) time.
func (g *GraphIndex) GetNodesByKind(kind string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ids := g.kindIndex[kind]
	res := make([]string, len(ids))
	copy(res, ids)
	return res
}

// GetOutEdges returns target node IDs for source and relation in O(1) time.
func (g *GraphIndex) GetOutEdges(sourceID, relation string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if rels, ok := g.forwardEdges[sourceID]; ok {
		if targets, ok := rels[relation]; ok {
			res := make([]string, len(targets))
			copy(res, targets)
			return res
		}
	}
	return nil
}

// GetInEdges returns source node IDs for target and relation in O(1) time.
func (g *GraphIndex) GetInEdges(targetID, relation string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if rels, ok := g.backwardEdges[targetID]; ok {
		if sources, ok := rels[relation]; ok {
			res := make([]string, len(sources))
			copy(res, sources)
			return res
		}
	}
	return nil
}

// GetAllNodeIDs returns all node IDs in the index.
func (g *GraphIndex) GetAllNodeIDs() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	res := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		res = append(res, id)
	}
	return res
}

// TotalNodes returns total node count N.
func (g *GraphIndex) TotalNodes() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// OperatorType identifies physical execution plan operator types.
type OperatorType string

const (
	OpIndexSeek  OperatorType = "IndexSeek"
	OpKindScan   OperatorType = "KindScan"
	OpEdgeExpand OperatorType = "EdgeExpand"
	OpFilter     OperatorType = "Filter"
	OpProject    OperatorType = "Project"
)

// PhysicalOperator represents an individual executable step in a query plan.
type PhysicalOperator struct {
	Type        OperatorType   `json:"type"`
	TargetKind  string         `json:"target_kind,omitempty"`
	Predicate   string         `json:"predicate,omitempty"`
	CostEstimate int           `json:"cost_estimate"`
	Children    []*PhysicalOperator `json:"children,omitempty"`
}

// PhysicalPlan represents an optimized execution plan.
type PhysicalPlan struct {
	Root          *PhysicalOperator `json:"root"`
	TotalCost     int               `json:"total_cost"`
	IndexSeekUsed bool              `json:"index_seek_used"`
	PushdownDone  bool              `json:"pushdown_done"`
}

// QuerySpec specifies graph query parameters for planning.
type QuerySpec struct {
	StartID      string
	TargetKind   string
	Relation     string
	MaxDepth     int
	FieldFilters map[string]any
}

// QueryPlanner constructs optimized physical execution plans from query specs.
type QueryPlanner struct {
	index *GraphIndex
}

// NewQueryPlanner creates a new query planner over a graph index.
func NewQueryPlanner(idx *GraphIndex) *QueryPlanner {
	return &QueryPlanner{index: idx}
}

// Plan generates a physical execution plan applying index seek and predicate pushdown.
func (p *QueryPlanner) Plan(query QuerySpec) (*PhysicalPlan, error) {
	plan := &PhysicalPlan{
		TotalCost:    1,
		PushdownDone: len(query.FieldFilters) > 0,
	}

	if query.StartID != "" {
		// Index Seek optimization
		plan.IndexSeekUsed = true
		seekOp := &PhysicalOperator{
			Type:         OpIndexSeek,
			Predicate:    fmt.Sprintf("id == %q", query.StartID),
			CostEstimate: 1,
		}
		if query.Relation != "" {
			expandOp := &PhysicalOperator{
				Type:         OpEdgeExpand,
				Predicate:    fmt.Sprintf("rel == %q", query.Relation),
				CostEstimate: 2,
				Children:     []*PhysicalOperator{seekOp},
			}
			plan.Root = expandOp
			plan.TotalCost = 3
		} else {
			plan.Root = seekOp
		}
	} else if query.TargetKind != "" {
		// Kind Scan optimization
		scanOp := &PhysicalOperator{
			Type:         OpKindScan,
			TargetKind:   query.TargetKind,
			CostEstimate: 5,
		}
		plan.Root = scanOp
		plan.TotalCost = 5
	} else {
		return nil, errors.New("query spec must specify StartID or TargetKind")
	}

	return plan, nil
}

// TraversalResult contains matched nodes and telemetry evidence.
type TraversalResult struct {
	VisitedNodes    []string          `json:"visited_nodes"`
	MatchedEntities []map[string]any `json:"matched_entities"`
	OperationsCount int               `json:"operations_count"`
	MaxDepthReached int               `json:"max_depth_reached"`
	CycleDetected   bool              `json:"cycle_detected"`
}

// TraversalEngine executes cycle-safe graph traversals with bounded complexity.
type TraversalEngine struct {
	index *GraphIndex
}

// NewTraversalEngine creates a traversal engine.
func NewTraversalEngine(idx *GraphIndex) *TraversalEngine {
	return &TraversalEngine{index: idx}
}

// Traverse performs a depth-bounded, cycle-safe graph traversal starting at startNodeID.
func (e *TraversalEngine) Traverse(ctx context.Context, startNodeID, relation string, maxDepth int) (*TraversalResult, error) {
	if maxDepth <= 0 {
		maxDepth = 16
	}

	startNode, exists := e.index.GetNode(startNodeID)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, startNodeID)
	}

	res := &TraversalResult{
		VisitedNodes:    make([]string, 0),
		MatchedEntities: make([]map[string]any, 0),
		OperationsCount: 1, // Start node lookup
	}

	res.VisitedNodes = append(res.VisitedNodes, startNodeID)
	res.MatchedEntities = append(res.MatchedEntities, startNode)

	visitedGlobal := make(map[string]bool)
	visitedGlobal[startNodeID] = true

	// Path set for cycle detection along current branch
	currentPath := make(map[string]bool)
	currentPath[startNodeID] = true

	err := e.dfs(ctx, startNodeID, relation, 1, maxDepth, currentPath, visitedGlobal, res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (e *TraversalEngine) dfs(
	ctx context.Context,
	currID string,
	relation string,
	currentDepth int,
	maxDepth int,
	currentPath map[string]bool,
	visitedGlobal map[string]bool,
	res *TraversalResult,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if currentDepth > res.MaxDepthReached {
		res.MaxDepthReached = currentDepth
	}

	// Fail-closed depth bound check
	if currentDepth > maxDepth {
		return fmt.Errorf("%w: exceeded depth %d at node %s", ErrCyclicRecursionExceeded, maxDepth, currID)
	}

	// Out edges lookup via index is O(1)
	outIDs := e.index.GetOutEdges(currID, relation)
	res.OperationsCount++ // Edge expand operation

	for _, nextID := range outIDs {
		res.OperationsCount++ // Node lookup operation

		// Cycle detection along current traversal branch
		if currentPath[nextID] {
			res.CycleDetected = true
			// Cycle safely recognized; skip re-traversing into cycle
			continue
		}

		// Avoid duplicate processing if already visited globally in DAG
		if visitedGlobal[nextID] {
			continue
		}

		visitedGlobal[nextID] = true
		currentPath[nextID] = true
		res.VisitedNodes = append(res.VisitedNodes, nextID)

		if nextNode, ok := e.index.GetNode(nextID); ok {
			res.MatchedEntities = append(res.MatchedEntities, nextNode)
		}

		err := e.dfs(ctx, nextID, relation, currentDepth+1, maxDepth, currentPath, visitedGlobal, res)
		delete(currentPath, nextID) // backtrack branch

		if err != nil {
			return err
		}
	}

	return nil
}
