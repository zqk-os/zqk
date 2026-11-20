// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrKnowledgeNodeNotFound is returned when a referenced knowledge node does not exist.
	ErrKnowledgeNodeNotFound = errors.New("knowledge node not found")
	// ErrDuplicateKnowledgeNode is returned when registering a node ID that already exists.
	ErrDuplicateKnowledgeNode = errors.New("knowledge node already registered")
	// ErrProtectedRootNode is returned when attempting to prune or compact a protected root compass node.
	ErrProtectedRootNode = errors.New("cannot prune or compact protected root compass node")
	// ErrCircularReference is returned when a cycle is detected under strict cycle checking.
	ErrCircularReference = errors.New("circular reference detected in lineage graph")
	// ErrInvalidThreshold is returned when semantic similarity threshold is outside (0, 1].
	ErrInvalidThreshold = errors.New("semantic similarity threshold must be in (0, 1]")
)

const defaultSimilarityThreshold = 0.95

// KnowledgeNode represents an entity in the knowledge lineage graph.
type KnowledgeNode struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	ContentHash string    `json:"content_hash,omitempty"`
	Embedding   []float64 `json:"embedding,omitempty"`
	IsRoot      bool      `json:"is_root"`
	Obsolete    bool      `json:"obsolete"`
	Refs        []string  `json:"refs,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// PruningReceipt summarizes the outcome of an autonomous orphan pruning pass.
type PruningReceipt struct {
	PrunedIDs     []string  `json:"pruned_ids"`
	RetainedIDs   []string  `json:"retained_ids"`
	CascadedEdges int       `json:"cascaded_edges"`
	ExecutedAt    time.Time `json:"executed_at"`
}

// CompactionReceipt summarizes the outcome of semantic deduping and obsolete node compaction.
type CompactionReceipt struct {
	CanonicalIDs    []string          `json:"canonical_ids"`
	CollapsedMap    map[string]string `json:"collapsed_map"`
	RemovedObsolete []string          `json:"removed_obsolete"`
	RewiredEdges    int               `json:"rewired_edges"`
	ExecutedAt      time.Time         `json:"executed_at"`
}

// KnowledgePrunerConfig configures graph validation, pruning, and semantic compaction.
type KnowledgePrunerConfig struct {
	StrictCycles        bool     `json:"strict_cycles"`
	SimilarityThreshold float64  `json:"similarity_threshold"`
	ProtectedRootIDs    []string `json:"protected_root_ids,omitempty"`
}

// KnowledgePruner manages autonomous knowledge pruning, semantic deduping, and graph compaction.
type KnowledgePruner struct {
	mu             sync.RWMutex
	config         KnowledgePrunerConfig
	nodes          map[string]*KnowledgeNode
	protectedRoots map[string]bool
}

// NewKnowledgePruner constructs a validated KnowledgePruner instance.
func NewKnowledgePruner(cfg KnowledgePrunerConfig) (*KnowledgePruner, error) {
	if cfg.SimilarityThreshold == 0 {
		cfg.SimilarityThreshold = defaultSimilarityThreshold
	}
	if cfg.SimilarityThreshold < 0 || cfg.SimilarityThreshold > 1 {
		return nil, ErrInvalidThreshold
	}

	protected := make(map[string]bool, len(cfg.ProtectedRootIDs))
	for _, id := range cfg.ProtectedRootIDs {
		if err := strutil.ErrorOnBlank(id, "protected root id cannot be blank"); err != nil {
			return nil, err
		}
		protected[id] = true
	}

	return &KnowledgePruner{
		config:         cfg,
		nodes:          make(map[string]*KnowledgeNode),
		protectedRoots: protected,
	}, nil
}

// RegisterNode validates and registers a knowledge node into the lineage graph.
func (kp *KnowledgePruner) RegisterNode(node KnowledgeNode) error {
	if err := validateNodeFields(node); err != nil {
		return err
	}

	kp.mu.Lock()
	defer kp.mu.Unlock()

	if _, exists := kp.nodes[node.ID]; exists {
		return ErrDuplicateKnowledgeNode
	}

	copied := cloneNode(node)
	if kp.protectedRoots[copied.ID] {
		copied.IsRoot = true
	}
	if copied.IsRoot {
		kp.protectedRoots[copied.ID] = true
	}

	kp.nodes[copied.ID] = &copied
	return nil
}

// ValidateGraph enforces static structural invariants across all registered nodes and edges.
func (kp *KnowledgePruner) ValidateGraph() error {
	kp.mu.RLock()
	defer kp.mu.RUnlock()

	for _, node := range kp.nodes {
		if err := validateNodeFields(*node); err != nil {
			return err
		}
	}

	if kp.config.StrictCycles {
		return kp.detectCyclesLocked()
	}
	return nil
}

// DetectOrphans computes unreachable node IDs from protected root compass nodes.
func (kp *KnowledgePruner) DetectOrphans() ([]string, error) {
	kp.mu.RLock()
	defer kp.mu.RUnlock()

	reachable, err := kp.computeReachableLocked()
	if err != nil {
		return nil, err
	}

	orphans := make([]string, 0)
	for id := range kp.nodes {
		if !reachable[id] && !kp.isRootLocked(id) {
			orphans = append(orphans, id)
		}
	}
	sort.Strings(orphans)
	return orphans, nil
}

// DeleteNode explicitly deletes a non-root node, rejecting protected root compass nodes.
func (kp *KnowledgePruner) DeleteNode(id string) error {
	if err := strutil.ErrorOnBlank(id, "node id cannot be blank"); err != nil {
		return err
	}

	kp.mu.Lock()
	defer kp.mu.Unlock()

	if _, exists := kp.nodes[id]; !exists {
		return ErrKnowledgeNodeNotFound
	}
	if kp.isRootLocked(id) {
		return ErrProtectedRootNode
	}

	delete(kp.nodes, id)
	kp.removeRefsToLocked(map[string]bool{id: true})
	return nil
}

// PruneOrphans removes unreachable non-root entities while preserving all reachable nodes intact.
func (kp *KnowledgePruner) PruneOrphans() (*PruningReceipt, error) {
	kp.mu.Lock()
	defer kp.mu.Unlock()

	reachable, err := kp.computeReachableLocked()
	if err != nil {
		return nil, err
	}

	toPrune := make(map[string]bool)
	retained := make([]string, 0, len(kp.nodes))
	for id := range kp.nodes {
		if reachable[id] || kp.isRootLocked(id) {
			retained = append(retained, id)
			continue
		}
		toPrune[id] = true
	}

	pruned := make([]string, 0, len(toPrune))
	cascadedEdges := 0
	for id := range toPrune {
		if kp.isRootLocked(id) {
			return nil, ErrProtectedRootNode
		}
		cascadedEdges += len(kp.nodes[id].Refs)
		delete(kp.nodes, id)
		pruned = append(pruned, id)
	}

	cascadedEdges += kp.removeRefsToLocked(toPrune)
	sort.Strings(pruned)
	sort.Strings(retained)

	return &PruningReceipt{
		PrunedIDs:     pruned,
		RetainedIDs:   retained,
		CascadedEdges: cascadedEdges,
		ExecutedAt:    time.Now().UTC(),
	}, nil
}

// CompactDuplicates collapses semantic duplicates and removes non-root obsolete nodes.
func (kp *KnowledgePruner) CompactDuplicates() (*CompactionReceipt, error) {
	kp.mu.Lock()
	defer kp.mu.Unlock()

	if kp.config.StrictCycles {
		if err := kp.detectCyclesLocked(); err != nil {
			return nil, err
		}
	}

	collapsedMap := kp.buildDuplicateMapLocked()
	rewiredEdges := kp.rewireDuplicateRefsLocked(collapsedMap)

	for dupID := range collapsedMap {
		if kp.isRootLocked(dupID) {
			return nil, ErrProtectedRootNode
		}
		delete(kp.nodes, dupID)
	}

	removedObsolete := kp.removeObsoleteNodesLocked()
	canonicalIDs := make([]string, 0, len(kp.nodes))
	for id := range kp.nodes {
		canonicalIDs = append(canonicalIDs, id)
	}
	sort.Strings(canonicalIDs)

	return &CompactionReceipt{
		CanonicalIDs:    canonicalIDs,
		CollapsedMap:    collapsedMap,
		RemovedObsolete: removedObsolete,
		RewiredEdges:    rewiredEdges,
		ExecutedAt:      time.Now().UTC(),
	}, nil
}

// SnapshotNodes returns a deep copy of all currently registered nodes.
func (kp *KnowledgePruner) SnapshotNodes() map[string]KnowledgeNode {
	kp.mu.RLock()
	defer kp.mu.RUnlock()

	snap := make(map[string]KnowledgeNode, len(kp.nodes))
	for id, node := range kp.nodes {
		snap[id] = cloneNode(*node)
	}
	return snap
}

func validateNodeFields(node KnowledgeNode) error {
	if err := strutil.ErrorOnBlank(node.ID, "node id cannot be blank"); err != nil {
		return err
	}
	for _, ref := range node.Refs {
		if err := strutil.ErrorOnBlank(ref, "node %s has blank edge target", node.ID); err != nil {
			return err
		}
	}
	return nil
}

func cloneNode(node KnowledgeNode) KnowledgeNode {
	cloned := node
	if node.Refs != nil {
		cloned.Refs = make([]string, len(node.Refs))
		copy(cloned.Refs, node.Refs)
	}
	if node.Embedding != nil {
		cloned.Embedding = make([]float64, len(node.Embedding))
		copy(cloned.Embedding, node.Embedding)
	}
	return cloned
}

func (kp *KnowledgePruner) isRootLocked(id string) bool {
	if kp.protectedRoots[id] {
		return true
	}
	node, exists := kp.nodes[id]
	return exists && node.IsRoot
}

func (kp *KnowledgePruner) detectCyclesLocked() error {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[string]int, len(kp.nodes))
	ids := kp.sortedIDsLocked()

	var dfs func(id string) error
	dfs = func(id string) error {
		state[id] = visiting
		node, exists := kp.nodes[id]
		if exists {
			for _, target := range node.Refs {
				if state[target] == visiting {
					return fmt.Errorf("%w: %s -> %s", ErrCircularReference, id, target)
				}
				if state[target] == unvisited {
					if err := dfs(target); err != nil {
						return err
					}
				}
			}
		}
		state[id] = visited
		return nil
	}

	for _, id := range ids {
		if state[id] == unvisited {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (kp *KnowledgePruner) computeReachableLocked() (map[string]bool, error) {
	if kp.config.StrictCycles {
		if err := kp.detectCyclesLocked(); err != nil {
			return nil, err
		}
	}

	reachable := make(map[string]bool, len(kp.nodes))
	queue := make([]string, 0)
	for _, id := range kp.sortedIDsLocked() {
		if kp.isRootLocked(id) {
			reachable[id] = true
			queue = append(queue, id)
		}
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		node, exists := kp.nodes[curr]
		if !exists {
			continue
		}
		for _, target := range node.Refs {
			if _, targetExists := kp.nodes[target]; targetExists && !reachable[target] {
				reachable[target] = true
				queue = append(queue, target)
			}
		}
	}
	return reachable, nil
}

func (kp *KnowledgePruner) removeRefsToLocked(removed map[string]bool) int {
	if len(removed) == 0 {
		return 0
	}
	removedCount := 0
	for _, node := range kp.nodes {
		if len(node.Refs) == 0 {
			continue
		}
		filtered := make([]string, 0, len(node.Refs))
		for _, ref := range node.Refs {
			if removed[ref] {
				removedCount++
				continue
			}
			filtered = append(filtered, ref)
		}
		node.Refs = filtered
	}
	return removedCount
}

func (kp *KnowledgePruner) buildDuplicateMapLocked() map[string]string {
	ids := kp.sortedIDsLocked()
	sort.SliceStable(ids, func(i, j int) bool {
		ni, nj := kp.nodes[ids[i]], kp.nodes[ids[j]]
		ri, rj := kp.isRootLocked(ni.ID), kp.isRootLocked(nj.ID)
		if ri != rj {
			return ri
		}
		if !ni.CreatedAt.Equal(nj.CreatedAt) {
			return ni.CreatedAt.Before(nj.CreatedAt)
		}
		return ni.ID < nj.ID
	})

	collapsed := make(map[string]string)
	survivors := make([]*KnowledgeNode, 0, len(ids))
	for _, id := range ids {
		candidate := kp.nodes[id]
		if kp.isRootLocked(id) || candidate.Obsolete {
			if kp.isRootLocked(id) {
				survivors = append(survivors, candidate)
			}
			continue
		}
		matchedID := kp.findSemanticMatch(candidate, survivors)
		if matchedID != "" {
			collapsed[id] = matchedID
			continue
		}
		survivors = append(survivors, candidate)
	}
	return collapsed
}

func (kp *KnowledgePruner) findSemanticMatch(candidate *KnowledgeNode, survivors []*KnowledgeNode) string {
	for _, canon := range survivors {
		if candidate.Kind != canon.Kind {
			continue
		}
		if candidate.ContentHash != "" && candidate.ContentHash == canon.ContentHash {
			return canon.ID
		}
		if len(candidate.Embedding) > 0 && len(candidate.Embedding) == len(canon.Embedding) {
			if CosineSimilarity(candidate.Embedding, canon.Embedding) >= kp.config.SimilarityThreshold {
				return canon.ID
			}
		}
	}
	return ""
}

func (kp *KnowledgePruner) rewireDuplicateRefsLocked(collapsed map[string]string) int {
	if len(collapsed) == 0 {
		return 0
	}
	rewired := 0
	for _, node := range kp.nodes {
		if len(node.Refs) == 0 {
			continue
		}
		seen := make(map[string]bool, len(node.Refs))
		updated := make([]string, 0, len(node.Refs))
		for _, ref := range node.Refs {
			target := ref
			if canon, ok := collapsed[ref]; ok {
				target = canon
				rewired++
			}
			if target == node.ID || seen[target] {
				continue
			}
			seen[target] = true
			updated = append(updated, target)
		}
		node.Refs = updated
	}
	return rewired
}

func (kp *KnowledgePruner) removeObsoleteNodesLocked() []string {
	obsoleteSet := make(map[string]bool)
	removed := make([]string, 0)
	for id, node := range kp.nodes {
		if node.Obsolete && !kp.isRootLocked(id) {
			obsoleteSet[id] = true
			removed = append(removed, id)
		}
	}
	for id := range obsoleteSet {
		delete(kp.nodes, id)
	}
	kp.removeRefsToLocked(obsoleteSet)
	sort.Strings(removed)
	return removed
}

func (kp *KnowledgePruner) sortedIDsLocked() []string {
	ids := make([]string, 0, len(kp.nodes))
	for id := range kp.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
