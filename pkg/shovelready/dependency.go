// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package shovelready

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ErrCycleDetected indicates that a cyclical dependency was detected in the backlog graph.
var ErrCycleDetected = errors.New("cyclical dependency detected in backlog graph")

// DependencyNode represents a single item and its dependency edges in the topology.
type DependencyNode struct {
	ID           string
	Dependencies []string
	IsBlocked    bool
	Status       string
}

// DependencyGraphAnalyzer provides thread-safe topological dependency validation.
type DependencyGraphAnalyzer struct {
	mu    sync.RWMutex
	nodes map[string]*DependencyNode
}

// NewDependencyGraphAnalyzer initializes an empty DependencyGraphAnalyzer.
func NewDependencyGraphAnalyzer() *DependencyGraphAnalyzer {
	return &DependencyGraphAnalyzer{
		nodes: make(map[string]*DependencyNode),
	}
}

// AddNode registers or updates a node with explicit dependencies and blocker state.
func (d *DependencyGraphAnalyzer) AddNode(id string, deps []string, isBlocked bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	cleanedDeps := make([]string, 0, len(deps))
	for _, dep := range deps {
		trimmed := strings.TrimSpace(dep)
		if trimmed != "" && !slices.Contains(cleanedDeps, trimmed) {
			cleanedDeps = append(cleanedDeps, trimmed)
		}
	}

	d.nodes[id] = &DependencyNode{
		ID:           id,
		Dependencies: cleanedDeps,
		IsBlocked:    isBlocked,
	}
}

// AddNodeFromBLI registers a node by extracting dependency references from a backlog item map.
func (d *DependencyGraphAnalyzer) AddNodeFromBLI(bli map[string]any) {
	if bli == nil {
		return
	}
	id, _ := bli[objects.FieldKeyID].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}

	deps := extractDependencyRefs(bli)
	blocked := hasUnresolvedBlocker(bli)

	d.mu.Lock()
	defer d.mu.Unlock()

	status, _ := bli[objects.FieldKeyStatus].(string)
	d.nodes[id] = &DependencyNode{
		ID:           id,
		Dependencies: deps,
		IsBlocked:    blocked,
		Status:       status,
	}
}

func extractDependencyRefs(bli map[string]any) []string {
	var deps []string
	candidateKeys := []string{
		"depends_on",
		"dependencies",
		"dependency_refs",
		objects.FieldKeyRiskBlockerRefs,
	}
	for _, key := range candidateKeys {
		val := bli[key]
		if val == nil {
			continue
		}
		switch v := val.(type) {
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed != "" && !slices.Contains(deps, trimmed) {
				deps = append(deps, trimmed)
			}
		case []string:
			for _, item := range v {
				trimmed := strings.TrimSpace(item)
				if trimmed != "" && !slices.Contains(deps, trimmed) {
					deps = append(deps, trimmed)
				}
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					trimmed := strings.TrimSpace(s)
					if trimmed != "" && !slices.Contains(deps, trimmed) {
						deps = append(deps, trimmed)
					}
				}
			}
		}
	}
	return deps
}

// RemoveNode unregisters a node by ID.
func (d *DependencyGraphAnalyzer) RemoveNode(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.nodes, id)
}

// NodeCount returns the number of registered nodes in the graph.
func (d *DependencyGraphAnalyzer) NodeCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.nodes)
}

// GetNode retrieves a registered node by ID.
func (d *DependencyGraphAnalyzer) GetNode(id string) (*DependencyNode, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	node, exists := d.nodes[id]
	if !exists {
		return nil, false
	}
	cp := *node
	return &cp, true
}

// ValidateAcyclic validates that the dependency graph has no circular dependencies.
// If a cycle exists, it returns ErrCycleDetected and the cycle path.
func (d *DependencyGraphAnalyzer) ValidateAcyclic() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	visited := make(map[string]int) // 0=unvisited, 1=visiting, 2=visited
	parent := make(map[string]string)

	var cyclePath []string
	var dfs func(u string) bool
	dfs = func(u string) bool {
		visited[u] = 1
		node := d.nodes[u]
		if node != nil {
			for _, v := range node.Dependencies {
				if _, ok := d.nodes[v]; !ok {
					continue
				}
				if visited[v] == 1 {
					cyclePath = reconstructCycle(u, v, parent)
					return true
				}
				if visited[v] == 0 {
					parent[v] = u
					if dfs(v) {
						return true
					}
				}
			}
		}
		visited[u] = 2
		return false
	}

	for id := range d.nodes {
		if visited[id] == 0 {
			if dfs(id) {
				return cyclePath, fmt.Errorf("%w: %s", ErrCycleDetected, strings.Join(cyclePath, " -> "))
			}
		}
	}

	return nil, nil
}

func reconstructCycle(start, end string, parent map[string]string) []string {
	path := []string{end, start}
	curr := start
	for curr != end {
		p, ok := parent[curr]
		if !ok || p == "" {
			break
		}
		path = append(path, p)
		curr = p
	}
	slices.Reverse(path)
	return path
}

// ResolveBlockedAncestors walks upstream dependencies for a node to determine if any ancestor is blocked.
// It returns a list of blocking ancestor IDs and a boolean indicating whether any blocker is active.
func (d *DependencyGraphAnalyzer) ResolveBlockedAncestors(id string) ([]string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var blockers []string
	visited := make(map[string]bool)

	var walk func(currID string)
	walk = func(currID string) {
		if visited[currID] {
			return
		}
		visited[currID] = true

		node, exists := d.nodes[currID]
		if !exists {
			return
		}

		if currID != id && (node.IsBlocked || strings.EqualFold(node.Status, objects.ObjectStatusBlocked)) {
			blockers = append(blockers, currID)
		}

		for _, dep := range node.Dependencies {
			walk(dep)
		}
	}

	walk(id)
	sort.Strings(blockers)
	return blockers, len(blockers) > 0
}

// HandleMissingReferences identifies all dependency references that are not present in knownIDs or graph nodes.
func (d *DependencyGraphAnalyzer) HandleMissingReferences(knownIDs map[string]struct{}) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	missingSet := make(map[string]struct{})
	for _, node := range d.nodes {
		for _, dep := range node.Dependencies {
			inKnown := false
			if knownIDs != nil {
				_, inKnown = knownIDs[dep]
			}
			_, inGraph := d.nodes[dep]
			if !inKnown && !inGraph {
				missingSet[dep] = struct{}{}
			}
		}
	}

	missing := make([]string, 0, len(missingSet))
	for k := range missingSet {
		missing = append(missing, k)
	}
	sort.Strings(missing)
	return missing
}

// GetTopologicalOrder computes a valid topological execution order (dependencies first).
func (d *DependencyGraphAnalyzer) GetTopologicalOrder() ([]string, error) {
	cyclePath, err := d.ValidateAcyclic()
	if err != nil {
		return nil, fmt.Errorf("cannot compute topological order: %w (path: %s)", err, strings.Join(cyclePath, " -> "))
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	visited := make(map[string]bool)
	var order []string

	var visit func(u string)
	visit = func(u string) {
		if visited[u] {
			return
		}
		visited[u] = true

		if node, exists := d.nodes[u]; exists {
			for _, dep := range node.Dependencies {
				if _, hasDep := d.nodes[dep]; hasDep {
					visit(dep)
				}
			}
		}
		order = append(order, u)
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(d.nodes))
	for k := range d.nodes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		visit(k)
	}

	return order, nil
}
