package storage

import (
	"encoding/json"
	"fmt"
	"maps"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// GraphHashRegistry manages hashes for all objects of a given kind in the graph backend
// Uses a single registry node per kind (e.g., "HashRegistry:backlog_item") that stores
// all hashes in a single property, matching the file-based HashRegistry pattern.
//
// Performance Characteristics:
// - Single node read/write per kind (same as file-based single file)
// - Bulk operations are efficient (one query to get all hashes)
// - Consistent with file-based HashRegistry interface
type GraphHashRegistry struct {
	kind       string
	conn       provider.GraphConnection
	hashes     map[string]string // identifier (object id) -> hash
	registryID string            // Graph node ID for the registry (e.g., "HashRegistry:backlog_item")
}

// NewGraphHashRegistry creates a new graph-based hash registry for a given object kind
func NewGraphHashRegistry(kind string, conn provider.GraphConnection) *GraphHashRegistry {
	// Registry node ID format: "HashRegistry:{kind}"
	// This matches the pattern of file-based registry: ".{kind}.hashes"
	registryID := fmt.Sprintf(ConstMiscHashregistryS, kind)
	return &GraphHashRegistry{
		kind:       kind,
		conn:       conn,
		hashes:     make(map[string]string),
		registryID: registryID,
	}
}

// Load loads the hash registry from the graph
func (ghr *GraphHashRegistry) Load() error {
	ctx := pkgctx.NewSystemContext()

	// Try to get the registry node
	node, err := ghr.conn.GetNode(ctx, ghr.registryID, []string{"HashRegistry"})
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToGetRegistryNode).Wrap(err)
	}

	if node == nil {
		// Registry doesn't exist yet - start with empty registry
		ghr.hashes = make(map[string]string)
		return nil
	}

	// Extract hashes from node properties
	hashesJSON, ok := node.Properties["hashes"].(string)
	if !ok {
		// No hashes property - start with empty registry
		ghr.hashes = make(map[string]string)
		return nil
	}

	// Parse JSON hashes
	var registry struct {
		Hashes map[string]string `json:"hashes"`
	}
	if err := json.Unmarshal([]byte(hashesJSON), &registry); err != nil {
		return errfmt.Newf(ConstMiscFailedToParseRegistryHashes).Wrap(err)
	}

	ghr.hashes = registry.Hashes
	if ghr.hashes == nil {
		ghr.hashes = make(map[string]string)
	}

	return nil
}

// Save saves the hash registry to the graph
func (ghr *GraphHashRegistry) Save() error {
	ctx := pkgctx.NewSystemContext()

	// Marshal hashes to JSON
	registry := struct {
		Hashes map[string]string `json:"hashes"`
	}{
		Hashes: ghr.hashes,
	}

	data, err := json.Marshal(registry)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToMarshalRegistry).Wrap(err)
	}

	// Check if registry node exists
	node, err := ghr.conn.GetNode(ctx, ghr.registryID, []string{"HashRegistry"})
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToCheckRegistryNode).Wrap(err)
	}

	if node == nil {
		// Create new registry node
		newNode := provider.Node{
			ID:     ghr.registryID,
			Labels: []string{"HashRegistry"},
			Properties: map[string]any{
				objects.FieldKeyKind: ghr.kind,
				"hashes":             string(data),
			},
		}
		return ghr.conn.CreateNode(ctx, newNode)
	}

	// Update existing registry node
	updates := provider.NodeUpdates{
		Properties: map[string]any{
			"hashes": string(data),
		},
	}
	return ghr.conn.UpdateNode(ctx, ghr.registryID, updates)
}

// GetHash returns the hash for a given object identifier
func (ghr *GraphHashRegistry) GetHash(identifier string) string {
	return ghr.hashes[identifier]
}

// SetHash sets the hash for a given object identifier
func (ghr *GraphHashRegistry) SetHash(identifier, hash string) {
	ghr.hashes[identifier] = hash
}

// DeleteHash removes the hash for a given object identifier
func (ghr *GraphHashRegistry) DeleteHash(identifier string) {
	delete(ghr.hashes, identifier)
}

// HasHash returns true if a hash exists for the given identifier
func (ghr *GraphHashRegistry) HasHash(identifier string) bool {
	_, exists := ghr.hashes[identifier]
	return exists
}

// GetAllHashes returns a copy of all hashes
func (ghr *GraphHashRegistry) GetAllHashes() map[string]string {
	result := make(map[string]string, len(ghr.hashes))
	maps.Copy(result, ghr.hashes)
	return result
}
