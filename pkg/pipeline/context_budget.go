package pipeline

import (
	"sort"
)

// ContextNode represents a single component of a budgeted context window.
type ContextNode struct {
	ID        string
	Tokens    int
	Weight    float64
	IsCore    bool     // True if this node was a direct hit
	PartOf    []string // IDs of parent structural objects
	DependsOn []string // IDs of dependencies
	Payload   any      // Opaque payload to carry through
}

// Allocator manages token budgeting hierarchically and topologically.
type Allocator struct {
	MaxTokens int
}

// NewAllocator creates a new token budget allocator.
func NewAllocator(maxTokens int) *Allocator {
	return &Allocator{MaxTokens: maxTokens}
}

// Allocate takes a set of candidates, applies the hierarchical budget,
// and returns the prioritized nodes topologically sorted.
func (a *Allocator) Allocate(candidates []ContextNode) []ContextNode {
	nodeMap := make(map[string]ContextNode)
	for _, c := range candidates {
		nodeMap[c.ID] = c
	}

	// 1. Identify Mandatory Structurals
	mandatoryIDs := make(map[string]bool)
	for _, c := range candidates {
		if c.IsCore {
			a.traverseDependencies(c, nodeMap, mandatoryIDs)
		}
	}

	for _, c := range candidates {
		if c.IsCore {
			delete(mandatoryIDs, c.ID)
		}
	}

	var mandatory []ContextNode
	var core []ContextNode
	var backfill []ContextNode

	for _, c := range candidates {
		if c.IsCore {
			core = append(core, c)
		} else if mandatoryIDs[c.ID] {
			mandatory = append(mandatory, c)
		} else {
			backfill = append(backfill, c)
		}
	}

	// Sort each tier by weight descending to prioritize within tier
	sortByWeightDesc := func(nodes []ContextNode) {
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].Weight > nodes[j].Weight
		})
	}
	sortByWeightDesc(mandatory)
	sortByWeightDesc(core)
	sortByWeightDesc(backfill)

	var selected []ContextNode
	budget := a.MaxTokens

	addNodes := func(nodes []ContextNode) {
		for _, n := range nodes {
			if n.Tokens <= budget {
				selected = append(selected, n)
				budget -= n.Tokens
			}
		}
	}

	addNodes(mandatory)
	addNodes(core)
	addNodes(backfill)

	return a.topologicalSort(selected)
}

func (a *Allocator) traverseDependencies(node ContextNode, nodeMap map[string]ContextNode, mandatoryIDs map[string]bool) {
	edges := append(node.PartOf, node.DependsOn...)
	for _, depID := range edges {
		if !mandatoryIDs[depID] {
			if _, exists := nodeMap[depID]; exists {
				mandatoryIDs[depID] = true
				a.traverseDependencies(nodeMap[depID], nodeMap, mandatoryIDs)
			}
		}
	}
}

// topologicalSort orders nodes so that dependencies appear before nodes that depend on them.
func (a *Allocator) topologicalSort(nodes []ContextNode) []ContextNode {
	selectedMap := make(map[string]ContextNode)
	for _, n := range nodes {
		selectedMap[n.ID] = n
	}

	var result []ContextNode
	visited := make(map[string]bool)
	tempMark := make(map[string]bool)

	var visit func(nodeID string)
	visit = func(nodeID string) {
		if tempMark[nodeID] {
			return
		}
		if !visited[nodeID] {
			tempMark[nodeID] = true
			node := selectedMap[nodeID]
			edges := append(node.PartOf, node.DependsOn...)
			for _, depID := range edges {
				if _, ok := selectedMap[depID]; ok {
					visit(depID)
				}
			}
			tempMark[nodeID] = false
			visited[nodeID] = true
			result = append(result, node)
		}
	}

	for _, n := range nodes {
		if !visited[n.ID] {
			visit(n.ID)
		}
	}

	return result
}
