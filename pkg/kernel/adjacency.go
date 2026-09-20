package kernel

// AdjacencyEngine and PlaneBoundaryEnforcer provide decoupled graph relations and
// plane isolation for the cellular microkernel architecture.
// Reference: BLI-CELLULAR-KERNEL-DECOUPLE-003

import (
	"context"
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/dna"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Edge represents a directed relation between two cellular entities identified by URN.
type Edge struct {
	Source     dna.URN        `json:"source"`
	Target     dna.URN        `json:"target"`
	Kind       string         `json:"kind"` // e.g. "causal", "depends_on", "implements", "relates_to"
	Weight     float64        `json:"weight"`
	Provenance dna.Provenance `json:"provenance"`
}

// ObjectEnvelope provides access to the BaseObject metadata of an entity.
type ObjectEnvelope interface {
	GetBaseObject() dna.BaseObject
}

// PlaneResolver resolves the operational plane for a given URN.
type PlaneResolver interface {
	ResolvePlane(ctx context.Context, urn dna.URN) (dna.Plane, error)
}

// PlaneMapResolver is an in-memory map-based PlaneResolver for fast testing and local execution.
type PlaneMapResolver struct {
	mu     sync.RWMutex
	planes map[string]dna.Plane
}

// NewPlaneMapResolver creates a new PlaneMapResolver.
func NewPlaneMapResolver() *PlaneMapResolver {
	return &PlaneMapResolver{
		planes: make(map[string]dna.Plane),
	}
}

// SetPlane sets the plane for a specific URN.
func (r *PlaneMapResolver) SetPlane(urn dna.URN, plane dna.Plane) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.planes[urn.String()] = plane
}

// ResolvePlane looks up the plane for a specific URN.
func (r *PlaneMapResolver) ResolvePlane(ctx context.Context, urn dna.URN) (dna.Plane, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plane, exists := r.planes[urn.String()]
	if !exists {
		return dna.PlaneDraft, errfmt.Errorf("plane not registered for urn %s", urn.String())
	}
	return plane, nil
}

// PlaneBoundaryEnforcer intercepts edge mutations to enforce plane isolation constraints.
type PlaneBoundaryEnforcer struct {
	resolver PlaneResolver
}

// NewPlaneBoundaryEnforcer instantiates a new PlaneBoundaryEnforcer.
func NewPlaneBoundaryEnforcer(resolver PlaneResolver) *PlaneBoundaryEnforcer {
	return &PlaneBoundaryEnforcer{
		resolver: resolver,
	}
}

// ValidateEdge enforces cellular isolation rules across plane boundaries:
// 1. Apoptotic Isolation: Neither Source nor Target can reside in PlaneApoptotic.
// 2. Draft Causal Isolation: An entity in PlaneDraft cannot link to PlanePromoted via causal/dependency edges without staging.
// 3. Upstream Contamination: An entity in PlanePromoted cannot have causal dependencies on PlaneDraft entities.
func (pbe *PlaneBoundaryEnforcer) ValidateEdge(ctx context.Context, edge Edge) error {
	if pbe.resolver == nil {
		return nil
	}

	srcPlane, err := pbe.resolver.ResolvePlane(ctx, edge.Source)
	if err != nil {
		return errfmt.Errorf("source plane resolution failed: %w", err)
	}

	tgtPlane, err := pbe.resolver.ResolvePlane(ctx, edge.Target)
	if err != nil {
		return errfmt.Errorf("target plane resolution failed: %w", err)
	}

	// 1. Apoptosis check
	if srcPlane == dna.PlaneApoptotic || tgtPlane == dna.PlaneApoptotic {
		return errfmt.Errorf("plane boundary violation: apoptotic objects cannot participate in active edges (src=%s[%s], tgt=%s[%s])",
			edge.Source.String(), srcPlane, edge.Target.String(), tgtPlane)
	}

	// 2. Draft causal boundary
	if edge.Kind == "causal" || edge.Kind == "depends_on" {
		if srcPlane == dna.PlaneDraft && tgtPlane == dna.PlanePromoted {
			return errfmt.Errorf("plane boundary violation: draft object (%s) cannot form causal dependency to promoted object (%s) without staging",
				edge.Source.String(), edge.Target.String())
		}
		if srcPlane == dna.PlanePromoted && srcPlane != tgtPlane && (tgtPlane == dna.PlaneDraft || tgtPlane == dna.PlaneStaged) {
			return errfmt.Errorf("plane boundary violation: promoted object (%s) cannot depend on unpromoted object (%s[%s])",
				edge.Source.String(), edge.Target.String(), tgtPlane)
		}
	}

	return nil
}

// AdjacencyEngine defines the core graph contract for cellular microkernel adjacency operations.
type AdjacencyEngine interface {
	AddEdge(ctx context.Context, edge Edge) error
	RemoveEdge(ctx context.Context, source, target dna.URN, kind string) error
	GetOutbound(ctx context.Context, source dna.URN) ([]Edge, error)
	GetInbound(ctx context.Context, target dna.URN) ([]Edge, error)
	DetectCycles(ctx context.Context) ([][]dna.URN, error)
	HasCycle(ctx context.Context) (bool, error)
}

// MemoryAdjacencyEngine is a thread-safe, in-memory implementation of AdjacencyEngine.
type MemoryAdjacencyEngine struct {
	mu       sync.RWMutex
	outbound map[string][]Edge
	inbound  map[string][]Edge
	enforcer *PlaneBoundaryEnforcer
}

// NewMemoryAdjacencyEngine constructs a new in-memory AdjacencyEngine.
func NewMemoryAdjacencyEngine(enforcer *PlaneBoundaryEnforcer) *MemoryAdjacencyEngine {
	return &MemoryAdjacencyEngine{
		outbound: make(map[string][]Edge),
		inbound:  make(map[string][]Edge),
		enforcer: enforcer,
	}
}

// AddEdge registers a new directed edge in the adjacency graph after plane validation.
func (m *MemoryAdjacencyEngine) AddEdge(ctx context.Context, edge Edge) error {
	if edge.Source.String() == "" || edge.Target.String() == "" {
		return errfmt.Errorf("invalid edge: source and target URNs must be non-empty")
	}

	if m.enforcer != nil {
		if err := m.enforcer.ValidateEdge(ctx, edge); err != nil {
			return err
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srcKey := edge.Source.String()
	tgtKey := edge.Target.String()

	// Check duplicate edge
	for _, existing := range m.outbound[srcKey] {
		if existing.Target.String() == tgtKey && existing.Kind == edge.Kind {
			return nil // Idempotent
		}
	}

	m.outbound[srcKey] = append(m.outbound[srcKey], edge)
	m.inbound[tgtKey] = append(m.inbound[tgtKey], edge)

	return nil
}

// RemoveEdge removes matching directed edges between source and target.
func (m *MemoryAdjacencyEngine) RemoveEdge(ctx context.Context, source, target dna.URN, kind string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	srcKey := source.String()
	tgtKey := target.String()

	filterOut := func(list []Edge) []Edge {
		var filtered []Edge
		for _, e := range list {
			if e.Target.String() == tgtKey && (kind == "" || e.Kind == kind) {
				continue
			}
			filtered = append(filtered, e)
		}
		return filtered
	}

	filterIn := func(list []Edge) []Edge {
		var filtered []Edge
		for _, e := range list {
			if e.Source.String() == srcKey && (kind == "" || e.Kind == kind) {
				continue
			}
			filtered = append(filtered, e)
		}
		return filtered
	}

	m.outbound[srcKey] = filterOut(m.outbound[srcKey])
	m.inbound[tgtKey] = filterIn(m.inbound[tgtKey])

	return nil
}

// GetOutbound returns all outbound edges originating from source.
func (m *MemoryAdjacencyEngine) GetOutbound(ctx context.Context, source dna.URN) ([]Edge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	edges, exists := m.outbound[source.String()]
	if !exists {
		return []Edge{}, nil
	}
	result := make([]Edge, len(edges))
	copy(result, edges)
	return result, nil
}

// GetInbound returns all inbound edges targeting target.
func (m *MemoryAdjacencyEngine) GetInbound(ctx context.Context, target dna.URN) ([]Edge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	edges, exists := m.inbound[target.String()]
	if !exists {
		return []Edge{}, nil
	}
	result := make([]Edge, len(edges))
	copy(result, edges)
	return result, nil
}

// DetectCycles returns all simple directed cycles found in the graph.
func (m *MemoryAdjacencyEngine) DetectCycles(ctx context.Context) ([][]dna.URN, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var cycles [][]dna.URN
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	currentPath := make([]dna.URN, 0)

	// Deterministic node iteration order
	nodes := make([]string, 0, len(m.outbound))
	for n := range m.outbound {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	var dfs func(nodeStr string)
	dfs = func(nodeStr string) {
		visited[nodeStr] = true
		recStack[nodeStr] = true

		u, _ := dna.ParseURN(nodeStr)
		currentPath = append(currentPath, u)

		for _, edge := range m.outbound[nodeStr] {
			tgtStr := edge.Target.String()
			if !visited[tgtStr] {
				dfs(tgtStr)
			} else if recStack[tgtStr] {
				// Cycle detected: extract subslice
				cycleStart := -1
				for i, node := range currentPath {
					if node.String() == tgtStr {
						cycleStart = i
						break
					}
				}
				if cycleStart != -1 {
					cycle := make([]dna.URN, len(currentPath[cycleStart:]))
					copy(cycle, currentPath[cycleStart:])
					cycles = append(cycles, cycle)
				}
			}
		}

		recStack[nodeStr] = false
		currentPath = currentPath[:len(currentPath)-1]
	}

	for _, n := range nodes {
		if !visited[n] {
			dfs(n)
		}
	}

	return cycles, nil
}

// HasCycle returns true if any cycle exists in the adjacency graph.
func (m *MemoryAdjacencyEngine) HasCycle(ctx context.Context) (bool, error) {
	cycles, err := m.DetectCycles(ctx)
	if err != nil {
		return false, err
	}
	return len(cycles) > 0, nil
}
