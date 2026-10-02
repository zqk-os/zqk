package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrCyclicGraphDetected  = errors.New("cyclic graph topology detected during traversal")
	ErrMaxRecursionExceeded = errors.New("maximum traversal recursion depth exceeded")
	ErrInvalidQueryPattern  = errors.New("invalid or empty query pattern")
	ErrUnsupportedOperator  = errors.New("unsupported physical operator")
)

// CyclePolicy defines how the traversal engine handles cyclic paths.
type CyclePolicy string

const (
	CyclePolicyFailClosed  CyclePolicy = "fail_closed"
	CyclePolicyDeduplicate CyclePolicy = "deduplicate"
)

// OperatorType identifies the physical plan execution step.
type OperatorType string

const (
	OpIndexSeek    OperatorType = "INDEX_SEEK"
	OpFullScan     OperatorType = "FULL_SCAN"
	OpTraverseEdge OperatorType = "TRAVERSE_EDGE"
	OpFilter       OperatorType = "PREDICATE_FILTER"
	OpProject      OperatorType = "PROJECT"
)

// Predicate represents a filtering constraint on node properties.
type Predicate struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` // "=", "!=", "IN", ">", "<"
	Value    any    `json:"value"`
}

// NodePattern describes a node matching element in a graph query AST.
type NodePattern struct {
	Variable   string      `json:"variable"`
	Kind       string      `json:"kind"`
	ID         string      `json:"id,omitempty"` // Specific anchor ID if provided
	Predicates []Predicate `json:"predicates,omitempty"`
}

// EdgePattern describes a relationship traversal element.
type EdgePattern struct {
	Variable  string `json:"variable,omitempty"`
	Relation  string `json:"relation"`
	Direction string `json:"direction"` // "outgoing", "incoming", "undirected"
	MinHops   int    `json:"min_hops"`
	MaxHops   int    `json:"max_hops"`
}

// QueryAST represents the parsed declarative ZPARQL query.
type QueryAST struct {
	StartNode  NodePattern     `json:"start_node"`
	Steps      []TraversalStep `json:"steps,omitempty"`
	Where      []Predicate     `json:"where,omitempty"`
	ReturnVars []string        `json:"return_vars"`
}

// TraversalStep represents an edge-node hop in a multi-hop pattern.
type TraversalStep struct {
	Edge EdgePattern `json:"edge"`
	Node NodePattern `json:"node"`
}

// PhysicalOperator represents an executable physical plan step.
type PhysicalOperator struct {
	Type          OperatorType      `json:"type"`
	TargetKind    string            `json:"target_kind,omitempty"`
	TargetID      string            `json:"target_id,omitempty"`
	Relation      string            `json:"relation,omitempty"`
	Direction     string            `json:"direction,omitempty"`
	Filter        []Predicate       `json:"filter,omitempty"`
	EstimatedCost float64           `json:"estimated_cost"`
	Next          *PhysicalOperator `json:"next,omitempty"`
}

// PhysicalPlan contains the root operator and metadata for query execution.
type PhysicalPlan struct {
	Root          *PhysicalOperator `json:"root"`
	EstimatedCost float64           `json:"estimated_cost"`
	PushdownDone  bool              `json:"pushdown_done"`
	Strategy      string            `json:"strategy"` // "index_accelerated" or "full_scan"
}

// QueryPlanner compiles a QueryAST into an optimized PhysicalPlan.
type QueryPlanner interface {
	Plan(ctx context.Context, query *QueryAST) (*PhysicalPlan, error)
}

// DefaultQueryPlanner implements rule-based and cost-based query planning.
type DefaultQueryPlanner struct{}

// NewQueryPlanner creates a new default query planner.
func NewQueryPlanner() *DefaultQueryPlanner {
	return &DefaultQueryPlanner{}
}

// Plan produces an optimized physical plan with predicate pushdown and index seek selection.
func (p *DefaultQueryPlanner) Plan(ctx context.Context, query *QueryAST) (*PhysicalPlan, error) {
	if query == nil {
		return nil, ErrInvalidQueryPattern
	}

	plan := &PhysicalPlan{
		Strategy: "index_accelerated",
	}

	var root *PhysicalOperator
	totalCost := 0.0

	// 1. Analyze Anchor Node (Start Node)
	// If ID is specified or an equality predicate exists on ID, use IndexSeek (O(1))
	anchorID := query.StartNode.ID
	pushedPredicates := make([]Predicate, 0)
	residualPredicates := make([]Predicate, 0)

	for _, pred := range query.Where {
		if pred.Field == "id" || pred.Field == query.StartNode.Variable+".id" {
			if pred.Operator == "=" {
				if strVal, ok := pred.Value.(string); ok {
					anchorID = strVal
				}
			}
		} else if strings.HasPrefix(pred.Field, query.StartNode.Variable+".") || pred.Field == "kind" {
			pushedPredicates = append(pushedPredicates, pred)
		} else {
			residualPredicates = append(residualPredicates, pred)
		}
	}

	if anchorID != "" {
		root = &PhysicalOperator{
			Type:          OpIndexSeek,
			TargetKind:    query.StartNode.Kind,
			TargetID:      anchorID,
			Filter:        pushedPredicates,
			EstimatedCost: 1.0,
		}
		totalCost += 1.0
	} else {
		// No specific index anchor -> full scan required
		plan.Strategy = "full_scan"
		root = &PhysicalOperator{
			Type:          OpFullScan,
			TargetKind:    query.StartNode.Kind,
			Filter:        pushedPredicates,
			EstimatedCost: 1000.0,
		}
		totalCost += 1000.0
	}

	current := root

	// 2. Multi-hop traversal steps
	for _, step := range query.Steps {
		stepPushed := make([]Predicate, 0)
		remResidual := make([]Predicate, 0)
		for _, pred := range residualPredicates {
			if strings.HasPrefix(pred.Field, step.Node.Variable+".") {
				stepPushed = append(stepPushed, pred)
			} else {
				remResidual = append(remResidual, pred)
			}
		}
		residualPredicates = remResidual

		hopOp := &PhysicalOperator{
			Type:          OpTraverseEdge,
			Relation:      step.Edge.Relation,
			Direction:     step.Edge.Direction,
			TargetKind:    step.Node.Kind,
			Filter:        stepPushed,
			EstimatedCost: 2.5,
		}
		totalCost += 2.5
		current.Next = hopOp
		current = hopOp
	}

	// 3. Residual Filter Operator if unpushed predicates remain
	if len(residualPredicates) > 0 {
		filterOp := &PhysicalOperator{
			Type:          OpFilter,
			Filter:        residualPredicates,
			EstimatedCost: 0.5 * float64(len(residualPredicates)),
		}
		totalCost += filterOp.EstimatedCost
		current.Next = filterOp
		current = filterOp
	}

	// 4. Projection Operator
	projOp := &PhysicalOperator{
		Type:          OpProject,
		EstimatedCost: 0.1,
	}
	totalCost += 0.1
	current.Next = projOp

	plan.Root = root
	plan.EstimatedCost = totalCost
	plan.PushdownDone = len(pushedPredicates) > 0
	return plan, nil
}

// IndexedGraphStorage provides an in-memory graph index mapping nodes and directed edges.
type IndexedGraphStorage struct {
	mu                  sync.RWMutex
	nodes               map[string]map[string]any
	outEdges            map[string][]Edge // sourceID -> edges
	inEdges             map[string][]Edge // targetID -> edges
	nodesInspectedCount int
	edgesInspectedCount int
}

// NewIndexedGraphStorage creates a thread-safe indexed graph storage.
func NewIndexedGraphStorage() *IndexedGraphStorage {
	return &IndexedGraphStorage{
		nodes:    make(map[string]map[string]any),
		outEdges: make(map[string][]Edge),
		inEdges:  make(map[string][]Edge),
	}
}

// AddNode adds a node to the indexed store.
func (s *IndexedGraphStorage) AddNode(id string, kind string, properties map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data := make(map[string]any)
	for k, v := range properties {
		data[k] = v
	}
	data["id"] = id
	data["kind"] = kind
	s.nodes[id] = data
}

// AddEdge adds a directed edge to forward and reverse indexes.
func (s *IndexedGraphStorage) AddEdge(sourceID, relation, targetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	edge := Edge{
		SourceID: sourceID,
		Relation: relation,
		TargetID: targetID,
	}
	s.outEdges[sourceID] = append(s.outEdges[sourceID], edge)
	s.inEdges[targetID] = append(s.inEdges[targetID], edge)
}

func (s *IndexedGraphStorage) incrementInspected(counter *int) {
	s.mu.Lock()
	*counter++
	s.mu.Unlock()
}

// GetNode retrieves a node by ID using the primary index.
func (s *IndexedGraphStorage) GetNode(id string) (map[string]any, bool) {
	s.incrementInspected(&s.nodesInspectedCount)

	s.mu.RLock()
	defer s.mu.RUnlock()
	n, exists := s.nodes[id]
	return n, exists
}

// GetOutboundEdges retrieves edges for a source node and relation via edge index.
func (s *IndexedGraphStorage) GetOutboundEdges(sourceID, relation string) []Edge {
	s.incrementInspected(&s.edgesInspectedCount)

	s.mu.RLock()
	defer s.mu.RUnlock()

	all := s.outEdges[sourceID]
	if relation == "" {
		return all
	}
	var res []Edge
	for _, e := range all {
		if e.Relation == relation {
			res = append(res, e)
		}
	}
	return res
}

// TotalNodes returns total node count in graph.
func (s *IndexedGraphStorage) TotalNodes() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodes)
}

// ResetMetrics resets inspection counters.
func (s *IndexedGraphStorage) ResetMetrics() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodesInspectedCount = 0
	s.edgesInspectedCount = 0
}

// Metrics returns nodes and edges inspected during execution.
func (s *IndexedGraphStorage) Metrics() (nodesInspected int, edgesInspected int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nodesInspectedCount, s.edgesInspectedCount
}

// TraversalOptions sets execution runtime controls.
type TraversalOptions struct {
	MaxDepth    int
	CyclePolicy CyclePolicy
}

// DefaultTraversalOptions returns default traversal settings.
func DefaultTraversalOptions() TraversalOptions {
	return TraversalOptions{
		MaxDepth:    16,
		CyclePolicy: CyclePolicyFailClosed,
	}
}

// QueryResult represents matched records.
type QueryResult struct {
	MatchedPaths [][]map[string]any `json:"matched_paths"`
	TotalMatched int                `json:"total_matched"`
}

// TraversalEngine executes physical plans against indexed storage.
type TraversalEngine struct {
	opts TraversalOptions
}

// NewTraversalEngine creates a new engine with given options.
func NewTraversalEngine(opts TraversalOptions) *TraversalEngine {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 16
	}
	if opts.CyclePolicy == "" {
		opts.CyclePolicy = CyclePolicyFailClosed
	}
	return &TraversalEngine{opts: opts}
}

// Execute runs the physical plan with cycle safety and depth bounding.
func (e *TraversalEngine) Execute(ctx context.Context, plan *PhysicalPlan, storage *IndexedGraphStorage) (*QueryResult, error) {
	if plan == nil || plan.Root == nil {
		return nil, ErrInvalidQueryPattern
	}

	root := plan.Root
	var initialCandidates []map[string]any

	switch root.Type {
	case OpIndexSeek:
		node, exists := storage.GetNode(root.TargetID)
		if !exists {
			return &QueryResult{MatchedPaths: nil, TotalMatched: 0}, nil
		}
		if root.TargetKind != "" && node["kind"] != root.TargetKind {
			return &QueryResult{MatchedPaths: nil, TotalMatched: 0}, nil
		}
		if !matchPredicates(node, root.Filter) {
			return &QueryResult{MatchedPaths: nil, TotalMatched: 0}, nil
		}
		initialCandidates = []map[string]any{node}

	case OpFullScan:
		storage.mu.RLock()
		for _, n := range storage.nodes {
			storage.nodesInspectedCount++
			if root.TargetKind == "" || n["kind"] == root.TargetKind {
				if matchPredicates(n, root.Filter) {
					initialCandidates = append(initialCandidates, n)
				}
			}
		}
		storage.mu.RUnlock()

	default:
		return nil, fmt.Errorf("%w: root op %s", ErrUnsupportedOperator, root.Type)
	}

	// Traverse remaining steps
	var paths [][]map[string]any
	for _, cand := range initialCandidates {
		initialPath := []map[string]any{cand}
		activePathIDs := map[string]bool{cand["id"].(string): true}

		subPaths, err := e.traverseStep(ctx, root.Next, initialPath, activePathIDs, 1, storage)
		if err != nil {
			return nil, err
		}
		paths = append(paths, subPaths...)
	}

	return &QueryResult{
		MatchedPaths: paths,
		TotalMatched: len(paths),
	}, nil
}

func (e *TraversalEngine) traverseStep(
	ctx context.Context,
	op *PhysicalOperator,
	currentPath []map[string]any,
	activePathIDs map[string]bool,
	currentDepth int,
	storage *IndexedGraphStorage,
) ([][]map[string]any, error) {
	if op == nil || op.Type == OpProject {
		return [][]map[string]any{currentPath}, nil
	}

	if currentDepth > e.opts.MaxDepth {
		return nil, fmt.Errorf("%w: depth %d exceeded max %d", ErrMaxRecursionExceeded, currentDepth, e.opts.MaxDepth)
	}

	switch op.Type {
	case OpTraverseEdge:
		lastNode := currentPath[len(currentPath)-1]
		sourceID := lastNode["id"].(string)

		edges := storage.GetOutboundEdges(sourceID, op.Relation)
		var resultPaths [][]map[string]any

		for _, edge := range edges {
			targetID := edge.TargetID

			// Cycle detection
			if activePathIDs[targetID] {
				if e.opts.CyclePolicy == CyclePolicyFailClosed {
					return nil, fmt.Errorf("%w: cycle encountered at node %s", ErrCyclicGraphDetected, targetID)
				}
				// Deduplicate / skip cycle edge
				continue
			}

			targetNode, exists := storage.GetNode(targetID)
			if !exists {
				continue
			}

			if op.TargetKind != "" && targetNode["kind"] != op.TargetKind {
				continue
			}

			if !matchPredicates(targetNode, op.Filter) {
				continue
			}

			// Extend path
			newPath := make([]map[string]any, len(currentPath), len(currentPath)+1)
			copy(newPath, currentPath)
			newPath = append(newPath, targetNode)

			newActiveIDs := make(map[string]bool, len(activePathIDs)+1)
			for k, v := range activePathIDs {
				newActiveIDs[k] = v
			}
			newActiveIDs[targetID] = true

			downstream, err := e.traverseStep(ctx, op.Next, newPath, newActiveIDs, currentDepth+1, storage)
			if err != nil {
				return nil, err
			}
			resultPaths = append(resultPaths, downstream...)
		}
		return resultPaths, nil

	case OpFilter:
		lastNode := currentPath[len(currentPath)-1]
		if !matchPredicates(lastNode, op.Filter) {
			return nil, nil
		}
		return e.traverseStep(ctx, op.Next, currentPath, activePathIDs, currentDepth, storage)

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedOperator, op.Type)
	}
}

func matchPredicates(node map[string]any, preds []Predicate) bool {
	for _, p := range preds {
		fieldName := p.Field
		if strings.Contains(fieldName, ".") {
			parts := strings.Split(fieldName, ".")
			fieldName = parts[len(parts)-1]
		}
		val, exists := node[fieldName]
		if !exists {
			return false
		}

		switch p.Operator {
		case "=":
			if fmt.Sprintf("%v", val) != fmt.Sprintf("%v", p.Value) {
				return false
			}
		case "!=":
			if fmt.Sprintf("%v", val) == fmt.Sprintf("%v", p.Value) {
				return false
			}
		}
	}
	return true
}
