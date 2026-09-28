package storage

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	// ErrPureGoNodeNotFound is returned when an indexed object cannot be found.
	ErrPureGoNodeNotFound = errors.New("purego_indexer: node not found in index")
	// ErrPureGoCyclicLimitExceeded is returned when graph traversal exceeds depth limit.
	ErrPureGoCyclicLimitExceeded = errors.New("purego_indexer: cyclic recursion or max depth exceeded")
)

// IndexedNode represents a normalized, searchable kernel object in the pure-Go index.
type IndexedNode struct {
	ID            string            `json:"id" gob:"id"`
	Kind          string            `json:"kind" gob:"kind"`
	SchemaVersion string            `json:"schema_version" gob:"schema_version"`
	Status        string            `json:"status" gob:"status"`
	Title         string            `json:"title" gob:"title"`
	NamespaceID   string            `json:"namespace_id" gob:"namespace_id"`
	CreatedAt     time.Time         `json:"created_at" gob:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at" gob:"updated_at"`
	Attributes    map[string]any    `json:"attributes" gob:"attributes"`
	References    map[string][]string `json:"references" gob:"references"` // relationName -> []targetIDs
}

// PureGoIndexer provides a thread-safe, pure-Go, CGO-free relational graph indexing layer
// that indexes CAS kernel objects on disk for sub-millisecond ZPARQL graph queries.
type PureGoIndexer struct {
	mu            sync.RWMutex
	nodes         map[string]*IndexedNode
	kindIndex     map[string]map[string]struct{}
	statusIndex   map[string]map[string]struct{}
	forwardEdges  map[string]map[string][]string // sourceID -> relation -> []targetID
	backwardEdges map[string]map[string][]string // targetID -> relation -> []sourceID
}

// NewPureGoIndexer constructs an empty, pure-Go in-memory graph index.
func NewPureGoIndexer() *PureGoIndexer {
	return &PureGoIndexer{
		nodes:         make(map[string]*IndexedNode),
		kindIndex:     make(map[string]map[string]struct{}),
		statusIndex:   make(map[string]map[string]struct{}),
		forwardEdges:  make(map[string]map[string][]string),
		backwardEdges: make(map[string]map[string][]string),
	}
}

// NodeCount returns the total number of indexed nodes.
func (p *PureGoIndexer) NodeCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.nodes)
}

// IndexNode registers or updates a node in the index.
func (p *PureGoIndexer) IndexNode(node *IndexedNode) error {
	if node == nil || strings.TrimSpace(node.ID) == "" {
		return errors.New("purego_indexer: node or node.ID cannot be empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// If node already exists, un-index old indices first
	if old, exists := p.nodes[node.ID]; exists {
		p.unindexNodeLocked(old)
	}

	// Clone node data to ensure isolation
	cloned := &IndexedNode{
		ID:            node.ID,
		Kind:          node.Kind,
		SchemaVersion: node.SchemaVersion,
		Status:        node.Status,
		Title:         node.Title,
		NamespaceID:   node.NamespaceID,
		CreatedAt:     node.CreatedAt,
		UpdatedAt:     node.UpdatedAt,
		Attributes:    make(map[string]any, len(node.Attributes)),
		References:    make(map[string][]string, len(node.References)),
	}
	for k, v := range node.Attributes {
		cloned.Attributes[k] = v
	}
	for k, v := range node.References {
		targets := make([]string, len(v))
		copy(targets, v)
		cloned.References[k] = targets
	}

	p.nodes[cloned.ID] = cloned

	// Index by Kind
	if cloned.Kind != "" {
		if p.kindIndex[cloned.Kind] == nil {
			p.kindIndex[cloned.Kind] = make(map[string]struct{})
		}
		p.kindIndex[cloned.Kind][cloned.ID] = struct{}{}
	}

	// Index by Status
	if cloned.Status != "" {
		if p.statusIndex[cloned.Status] == nil {
			p.statusIndex[cloned.Status] = make(map[string]struct{})
		}
		p.statusIndex[cloned.Status][cloned.ID] = struct{}{}
	}

	// Index References / Edges
	for relation, targets := range cloned.References {
		for _, targetID := range targets {
			if targetID == "" {
				continue
			}
			// Forward edge
			if p.forwardEdges[cloned.ID] == nil {
				p.forwardEdges[cloned.ID] = make(map[string][]string)
			}
			p.forwardEdges[cloned.ID][relation] = append(p.forwardEdges[cloned.ID][relation], targetID)

			// Backward edge
			if p.backwardEdges[targetID] == nil {
				p.backwardEdges[targetID] = make(map[string][]string)
			}
			p.backwardEdges[targetID][relation] = append(p.backwardEdges[targetID][relation], cloned.ID)
		}
	}

	return nil
}

// RemoveNode removes a node and its indexed edges from the index.
func (p *PureGoIndexer) RemoveNode(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	node, exists := p.nodes[id]
	if !exists {
		return ErrPureGoNodeNotFound
	}
	p.unindexNodeLocked(node)
	delete(p.nodes, id)
	return nil
}

func (p *PureGoIndexer) unindexNodeLocked(node *IndexedNode) {
	if node.Kind != "" && p.kindIndex[node.Kind] != nil {
		delete(p.kindIndex[node.Kind], node.ID)
		if len(p.kindIndex[node.Kind]) == 0 {
			delete(p.kindIndex, node.Kind)
		}
	}
	if node.Status != "" && p.statusIndex[node.Status] != nil {
		delete(p.statusIndex[node.Status], node.ID)
		if len(p.statusIndex[node.Status]) == 0 {
			delete(p.statusIndex, node.Status)
		}
	}

	// Remove forward edges from this node
	if rels, ok := p.forwardEdges[node.ID]; ok {
		for rel, targets := range rels {
			for _, targetID := range targets {
				if bRels, bOk := p.backwardEdges[targetID]; bOk {
					bRels[rel] = removeStringFromSlice(bRels[rel], node.ID)
					if len(bRels[rel]) == 0 {
						delete(bRels, rel)
					}
					if len(bRels) == 0 {
						delete(p.backwardEdges, targetID)
					}
				}
			}
		}
		delete(p.forwardEdges, node.ID)
	}

	// Remove backward edges pointing to this node
	if bRels, ok := p.backwardEdges[node.ID]; ok {
		for rel, sources := range bRels {
			for _, sourceID := range sources {
				if fRels, fOk := p.forwardEdges[sourceID]; fOk {
					fRels[rel] = removeStringFromSlice(fRels[rel], node.ID)
					if len(fRels[rel]) == 0 {
						delete(fRels, rel)
					}
					if len(fRels) == 0 {
						delete(p.forwardEdges, sourceID)
					}
				}
			}
		}
		delete(p.backwardEdges, node.ID)
	}
}

// GetNode retrieves a node by its ID in O(1) time.
func (p *PureGoIndexer) GetNode(id string) (*IndexedNode, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	node, ok := p.nodes[id]
	if !ok {
		return nil, false
	}
	return node, true
}

// GetNodesByKind returns all nodes for a specific kind in O(K) time.
func (p *PureGoIndexer) GetNodesByKind(kind string) []*IndexedNode {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids, ok := p.kindIndex[kind]
	if !ok || len(ids) == 0 {
		return nil
	}

	result := make([]*IndexedNode, 0, len(ids))
	for id := range ids {
		if node, exists := p.nodes[id]; exists {
			result = append(result, node)
		}
	}
	return result
}

// GetNodesByStatus returns all nodes matching a lifecycle status in O(S) time.
func (p *PureGoIndexer) GetNodesByStatus(status string) []*IndexedNode {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids, ok := p.statusIndex[status]
	if !ok || len(ids) == 0 {
		return nil
	}

	result := make([]*IndexedNode, 0, len(ids))
	for id := range ids {
		if node, exists := p.nodes[id]; exists {
			result = append(result, node)
		}
	}
	return result
}

// GetOutEdges returns the target node IDs referenced by sourceID via relation in O(1) time.
func (p *PureGoIndexer) GetOutEdges(sourceID, relation string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rels, ok := p.forwardEdges[sourceID]
	if !ok {
		return nil
	}
	targets, ok := rels[relation]
	if !ok || len(targets) == 0 {
		return nil
	}
	res := make([]string, len(targets))
	copy(res, targets)
	return res
}

// GetInEdges returns the source node IDs referencing targetID via relation in O(1) time.
func (p *PureGoIndexer) GetInEdges(targetID, relation string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rels, ok := p.backwardEdges[targetID]
	if !ok {
		return nil
	}
	sources, ok := rels[relation]
	if !ok || len(sources) == 0 {
		return nil
	}
	res := make([]string, len(sources))
	copy(res, sources)
	return res
}

// Traverse executes a multi-hop graph path traversal starting from startID following a slice of relations.
// For example: path = ["milestone_refs", "goal_refs", "vision_refs"].
// Max depth ensures safe bounds against circular references.
func (p *PureGoIndexer) Traverse(ctx context.Context, startID string, path []string, maxDepth int) ([]*IndexedNode, error) {
	if maxDepth <= 0 {
		maxDepth = 32
	}
	if len(path) == 0 {
		if node, ok := p.GetNode(startID); ok {
			return []*IndexedNode{node}, nil
		}
		return nil, ErrPureGoNodeNotFound
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	currentIDs := []string{startID}

	for hopIdx, relation := range path {
		if hopIdx >= maxDepth {
			return nil, ErrPureGoCyclicLimitExceeded
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		nextIDs := make([]string, 0)
		seenInHop := make(map[string]bool)
		for _, curr := range currentIDs {
			if rels, ok := p.forwardEdges[curr]; ok {
				if targets, tOk := rels[relation]; tOk {
					for _, target := range targets {
						if !seenInHop[target] {
							seenInHop[target] = true
							nextIDs = append(nextIDs, target)
						}
					}
				}
			}
		}

		if len(nextIDs) == 0 {
			return nil, nil
		}
		currentIDs = nextIDs
	}

	nodes := make([]*IndexedNode, 0, len(currentIDs))
	for _, id := range currentIDs {
		if n, ok := p.nodes[id]; ok {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

// TraverseRecursive traverses edges following relation repeatedly until no new nodes are discovered or maxDepth is reached.
// Visited nodes are deduplicated to prevent cyclic infinite loops.
func (p *PureGoIndexer) TraverseRecursive(ctx context.Context, startID string, relation string, maxDepth int) ([]*IndexedNode, error) {
	if maxDepth <= 0 {
		maxDepth = 64
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	visited := make(map[string]bool)
	queue := []string{startID}
	visited[startID] = true
	var results []*IndexedNode

	depth := 0
	for len(queue) > 0 {
		if depth >= maxDepth {
			return results, ErrPureGoCyclicLimitExceeded
		}
		depth++

		levelSize := len(queue)
		for i := 0; i < levelSize; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			curr := queue[0]
			queue = queue[1:]

			if rels, ok := p.forwardEdges[curr]; ok {
				if targets, tOk := rels[relation]; tOk {
					for _, targetID := range targets {
						if !visited[targetID] {
							visited[targetID] = true
							queue = append(queue, targetID)
							if n, ok := p.nodes[targetID]; ok {
								results = append(results, n)
							}
						}
					}
				}
			}
		}
	}

	return results, nil
}

// IndexProjectDir walks projectRoot/.zqk/data (and related storage directories) and indexes
// all valid YAML objects into the pure-Go indexer.
func (p *PureGoIndexer) IndexProjectDir(ctx context.Context, projectRoot string) (int, error) {
	dataDir := filepath.Join(projectRoot, paths.ProjectDataDir, "data")
	if fi, err := fileutil.Stat(dataDir); err != nil || !fi.IsDir() {
		// Fallback to checking paths.ProjectDataDir
		dataDir = filepath.Join(projectRoot, paths.ProjectDataDir)
		if fi, err := fileutil.Stat(dataDir); err != nil || !fi.IsDir() {
			return 0, nil
		}
	}

	indexedCount := 0
	err := filepath.WalkDir(dataDir, func(path string, d fileutil.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "cache" || name == "logs" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := fileutil.ReadFile(path)
		if err != nil {
			return nil
		}

		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err != nil || len(obj) == 0 {
			return nil
		}

		id, _ := obj["id"].(string)
		kind, _ := obj["kind"].(string)
		if id == "" || kind == "" {
			return nil
		}

		node := parseIndexedNodeFromMap(id, kind, obj)
		if err := p.IndexNode(node); err == nil {
			indexedCount++
		}
		return nil
	})

	return indexedCount, err
}

// SaveSnapshot serializes the entire index state into a binary gob format.
func (p *PureGoIndexer) SaveSnapshot(w io.Writer) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	enc := gob.NewEncoder(w)
	nodesList := make([]*IndexedNode, 0, len(p.nodes))
	for _, n := range p.nodes {
		nodesList = append(nodesList, n)
	}

	return enc.Encode(nodesList)
}

// LoadSnapshot deserializes index state from a binary gob format.
func (p *PureGoIndexer) LoadSnapshot(r io.Reader) error {
	dec := gob.NewDecoder(r)
	var nodesList []*IndexedNode
	if err := dec.Decode(&nodesList); err != nil {
		return fmt.Errorf("purego_indexer: failed to decode snapshot: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.nodes = make(map[string]*IndexedNode, len(nodesList))
	p.kindIndex = make(map[string]map[string]struct{})
	p.statusIndex = make(map[string]map[string]struct{})
	p.forwardEdges = make(map[string]map[string][]string)
	p.backwardEdges = make(map[string]map[string][]string)

	for _, node := range nodesList {
		p.nodes[node.ID] = node

		if node.Kind != "" {
			if p.kindIndex[node.Kind] == nil {
				p.kindIndex[node.Kind] = make(map[string]struct{})
			}
			p.kindIndex[node.Kind][node.ID] = struct{}{}
		}

		if node.Status != "" {
			if p.statusIndex[node.Status] == nil {
				p.statusIndex[node.Status] = make(map[string]struct{})
			}
			p.statusIndex[node.Status][node.ID] = struct{}{}
		}

		for rel, targets := range node.References {
			for _, tID := range targets {
				if tID == "" {
					continue
				}
				if p.forwardEdges[node.ID] == nil {
					p.forwardEdges[node.ID] = make(map[string][]string)
				}
				p.forwardEdges[node.ID][rel] = append(p.forwardEdges[node.ID][rel], tID)

				if p.backwardEdges[tID] == nil {
					p.backwardEdges[tID] = make(map[string][]string)
				}
				p.backwardEdges[tID][rel] = append(p.backwardEdges[tID][rel], node.ID)
			}
		}
	}

	return nil
}

func parseIndexedNodeFromMap(id, kind string, obj map[string]any) *IndexedNode {
	node := &IndexedNode{
		ID:         id,
		Kind:       kind,
		Attributes: make(map[string]any),
		References: make(map[string][]string),
	}

	if sv, ok := obj["schema_version"].(string); ok {
		node.SchemaVersion = sv
	}
	if st, ok := obj["status"].(string); ok {
		node.Status = st
	}
	if t, ok := obj["title"].(string); ok {
		node.Title = t
	}
	if ns, ok := obj["namespace_id"].(string); ok {
		node.NamespaceID = ns
	}
	if ca, ok := obj["created_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ca); err == nil {
			node.CreatedAt = parsed
		}
	}
	if ua, ok := obj["updated_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ua); err == nil {
			node.UpdatedAt = parsed
		}
	}

	for k, v := range obj {
		switch k {
		case "id", "kind", "schema_version", "status", "title", "namespace_id", "created_at", "updated_at":
			continue
		}

		// Detect references: fields ending in _ref or _refs
		if strings.HasSuffix(k, "_refs") {
			if list, ok := v.([]any); ok {
				for _, item := range list {
					if refStr, ok := item.(string); ok && refStr != "" {
						node.References[k] = append(node.References[k], refStr)
					}
				}
			} else if list, ok := v.([]string); ok {
				node.References[k] = append(node.References[k], list...)
			}
		} else if strings.HasSuffix(k, "_ref") {
			if refStr, ok := v.(string); ok && refStr != "" {
				node.References[k] = append(node.References[k], refStr)
			}
		} else {
			node.Attributes[k] = v
		}
	}

	return node
}

func removeStringFromSlice(slice []string, val string) []string {
	out := slice[:0]
	for _, s := range slice {
		if s != val {
			out = append(out, s)
		}
	}
	return out
}

func uniqueStrings(slice []string) []string {
	seen := make(map[string]bool, len(slice))
	out := make([]string, 0, len(slice))
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func init() {
	gob.Register(map[string]any{})
	gob.Register([]any{})
	gob.Register([]string{})
	gob.Register(time.Time{})
}
