package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Exists checks if an object exists by ID
// More efficient than Read() as it doesn't load the object
func (g *GraphObjectStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	// Infer kind from ID
	if err := g.idValidator.LoadPatterns(); err != nil {
		return false, errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	kind := g.idValidator.InferKindFromID(id)
	if kind == emptyValue {
		return false, errfmt.Errorf(ConstStreamCouldNotInferKindFromIdStr, id)
	}

	// Check permission
	if err := g.checkPermission(secCtx, "read", kind); err != nil {
		return false, err
	}

	// Check if node exists (more efficient than reading full node)
	label := toLabel(kind)
	node, err := g.conn.GetNode(ctx, id, []string{label, "Entity"})
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.Exists GetNode", err).Log()
		}
		// If error is "not found", return false
		gerr := &provider.GraphError{}
		if errors.As(err, &gerr) {
			return false, nil
		}
		// Error means query failed - this is a real error, not "not found"
		return false, errfmt.Newf(ConstStreamFailedToCheckIfObjectExists).Wrap(err)
	}
	return node != nil, nil
}

// Count counts objects matching the filter
// More efficient than List() as it uses COUNT() in Cypher without loading objects
//
//nolint:gocritic // Interface requires value semantics for ListFilter
func (g *GraphObjectStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	// Check permission
	if err := g.checkPermission(secCtx, "read", filter.Kind); err != nil {
		return 0, err
	}

	// Build Cypher query with COUNT()
	label := toLabel(filter.Kind)
	query := fmt.Sprintf(ConstStreamMatchNStrEntity, label)

	// Build parameters map
	params := make(map[string]any)

	// Add filters with support for operators
	if len(filter.Filters) > 0 {
		conditions := []string{}

		for field, filterValue := range filter.Filters {
			// Handle filter with operator (map syntax)
			if filterMap, ok := filterValue.(map[string]any); ok {
				for opStr, opValue := range filterMap {
					operator := FilterOperator(opStr)
					cleanOp := strings.ReplaceAll(opStr, "$", "")
					paramName := fmt.Sprintf("filter_%s_%s", field, cleanOp)
					condition := g.buildCypherCondition(field, operator, paramName)
					if condition != emptyValue {
						conditions = append(conditions, condition)
						params[paramName] = opValue
					}
				}
			} else {
				// Simple equality (backward compatible)
				paramName := fmt.Sprintf("filter_%s", field)
				conditions = append(conditions, fmt.Sprintf("n.%s = $%s", field, paramName))
				params[paramName] = filterValue
			}
		}

		if len(conditions) > 0 {
			query += " WHERE " + strings.Join(conditions, " AND ")
		}
	}

	// Use COUNT() instead of RETURN n
	query += ConstStreamReturnCountNAsCount

	// Execute query using ExecuteQuery
	cypherQuery := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    query,
		Params:   params,
	}

	queryResult, err := g.conn.ExecuteQuery(ctx, cypherQuery)
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.Count ExecuteQuery", err).Log()
		}
		return 0, errfmt.Newf(ConstStreamFailedToExecuteCountQuery).Wrap(err)
	}

	// Extract count from result
	// ExecuteQuery returns QueryResult with Rows
	if len(queryResult.Rows) == 0 {
		return 0, nil
	}

	// Get count value from first row
	firstRow := queryResult.Rows[0]
	countValue, ok := firstRow["count"]
	if !ok {
		return 0, errfmt.Errorf(ConstStreamCountQueryDidNotReturnCountField)
	}

	// Handle different numeric types
	switch v := countValue.(type) {
	case int64:
		return int(v), nil
	case int:
		return v, nil
	case float64:
		return int(v), nil
	default:
		return 0, errfmt.Errorf(ConstStreamUnexpectedCountValueTypeType, countValue)
	}
}

// GetRelated finds objects related to the given object via reference fields
func (g *GraphObjectStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id, relationshipType string, depth int) ([]map[string]any, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	// Read the starting object to get its kind
	startObj, err := g.Read(ctx, secCtx, id)
	if err != nil {
		return nil, errfmt.Errorf(ConstStreamFailedToReadObjectStrErr, id, err)
	}

	kind, _ := startObj[objects.FieldKeyKind].(string)
	if kind == emptyValue {
		return nil, errfmt.Errorf(ConstStreamObjectStrMissingKindField, id)
	}

	// Convert relationship type to relationship name for graph traversal
	// If relationshipType is empty, traverse all relationships
	relationship := ""
	if relationshipType != emptyValue {
		// Convert field name to relationship type (e.g., "priority_plan_ref" -> "PRIORITY_PLAN_REF")
		relationship = strings.ToUpper(relationshipType)
	}

	// Build traversal query
	traversal := provider.TraversalQuery{
		StartNodeID:  id,
		Relationship: relationship,
		Direction:    provider.DirectionOutgoing,
		MaxDepth:     depth,
		Filter:       provider.NodeFilter{},
	}

	// Execute traversal
	result, err := g.conn.ExecuteTraversal(ctx, traversal)
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.GetRelated ExecuteTraversal", err).Log()
		}
		return nil, errfmt.Newf(ConstStreamTraversalFailed).Wrap(err)
	}

	// Convert nodes to objects and check permissions
	related := make([]map[string]any, 0)
	for _, node := range result.Nodes {
		obj := g.nodeToObject(node)
		if obj == nil {
			continue
		}

		// Check permissions
		objKind, _ := obj[objects.FieldKeyKind].(string)
		if err := g.checkPermission(secCtx, "read", objKind); err != nil {
			continue // Skip objects we don't have permission to read
		}

		related = append(related, obj)
	}

	return related, nil
}

// GetPath finds a path between two objects via reference relationships
func (g *GraphObjectStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	// Use BFS traversal to find path
	// This works with the current provider interface
	// Future optimization: use Cypher shortestPath query when path extraction is available
	return g.findPathBFS(ctx, secCtx, fromID, toID)
}

const (
	maxBFSDepth     = 32
	maxBFSQueueSize = 5000
)

// findPathBFS finds a path between two nodes using BFS
func (g *GraphObjectStorage) findPathBFS(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	type pathNode struct {
		id   string
		path []string
	}

	queue := []pathNode{{id: fromID, path: []string{fromID}}}
	visited := make(map[string]bool)
	visited[fromID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Enforce depth bounds to prevent unbounded queue growth on large or cyclical graphs (BLI-CEF-PERF-003)
		if len(current.path) >= maxBFSDepth {
			continue
		}

		// Get neighbors
		neighbors, err := g.GetNeighbors(ctx, secCtx, current.id, "outgoing")
		if err != nil {
			continue
		}

		for _, neighbor := range neighbors {
			neighborID, _ := neighbor[objects.FieldKeyID].(string)
			if neighborID == emptyValue {
				continue
			}

			if neighborID == toID {
				// Found path - build result
				path := make([]string, len(current.path))
				copy(path, current.path)
				path = append(path, toID)

				// Read all objects in path
				result := make([]map[string]any, 0, len(path))
				for _, pathID := range path {
					obj, err := g.Read(ctx, secCtx, pathID)
					if err != nil {
						return nil, errfmt.Errorf(ConstStreamFailedToReadObjectStrInPathErr, pathID, err)
					}
					result = append(result, obj)
				}
				return result, nil
			}

			if !visited[neighborID] && len(queue) < maxBFSQueueSize {
				visited[neighborID] = true
				newPath := make([]string, len(current.path))
				copy(newPath, current.path)
				newPath = append(newPath, neighborID)
				queue = append(queue, pathNode{id: neighborID, path: newPath})
			}
		}
	}

	// No path found
	return []map[string]any{}, nil
}

// GetNeighbors finds immediate neighbors of an object (depth=1)
func (g *GraphObjectStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	// Read the object to verify it exists
	_, err := g.Read(ctx, secCtx, id)
	if err != nil {
		return nil, errfmt.Errorf(ConstStreamFailedToReadObjectStrErr, id, err)
	}

	// Attempt single bulk traversal query first to eliminate N+1 roundtrips (BLI-CEF-PERF-001)
	var travDir provider.Direction
	switch direction {
	case "outgoing":
		travDir = provider.DirectionOutgoing
	case "incoming":
		travDir = provider.DirectionIncoming
	case "both":
		travDir = provider.DirectionBoth
	}

	neighbors := make([]map[string]any, 0)
	if travDir != "" {
		traversal := provider.TraversalQuery{
			StartNodeID: id,
			Direction:   travDir,
			MaxDepth:    1,
			Filter:      provider.NodeFilter{},
		}
		if res, travErr := g.conn.ExecuteTraversal(ctx, traversal); travErr == nil && res != nil && len(res.Nodes) > 0 {
			seen := make(map[string]bool)
			for _, node := range res.Nodes {
				if node == nil || node.ID == id || seen[node.ID] {
					continue
				}
				seen[node.ID] = true
				obj := g.nodeToObject(node)
				if obj != nil {
					objKind, _ := obj[objects.FieldKeyKind].(string)
					if err := g.checkPermission(secCtx, "read", objKind); err == nil {
						neighbors = append(neighbors, obj)
					}
				}
			}
			return neighbors, nil
		}
	}

	// Fallback to edge listing with deduplicated node lookups
	seenNodes := make(map[string]bool)
	var targetIDs []string

	// Handle outgoing (objects this references)
	if direction == "outgoing" || direction == "both" {
		edgeFilter := provider.EdgeFilter{
			FromID: id,
			Limit:  1000,
		}

		edges, err := g.conn.ListEdges(ctx, edgeFilter)
		if err != nil && isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.GetNeighbors ListEdges", err).Log()
		}
		if err == nil {
			for _, edge := range edges {
				if edge != nil && edge.ToID != "" && edge.ToID != id && !seenNodes[edge.ToID] {
					seenNodes[edge.ToID] = true
					targetIDs = append(targetIDs, edge.ToID)
				}
			}
		}
	}

	// Handle incoming (objects that reference this)
	if direction == "incoming" || direction == "both" {
		edgeFilter := provider.EdgeFilter{
			ToID:  id,
			Limit: 1000,
		}

		edges, err := g.conn.ListEdges(ctx, edgeFilter)
		if err != nil && isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.GetNeighbors ListEdges", err).Log()
		}
		if err == nil {
			for _, edge := range edges {
				if edge != nil && edge.FromID != "" && edge.FromID != id && !seenNodes[edge.FromID] {
					seenNodes[edge.FromID] = true
					targetIDs = append(targetIDs, edge.FromID)
				}
			}
		}
	}

	for _, nodeID := range targetIDs {
		node, readErr := g.conn.GetNode(ctx, nodeID, nil)
		if readErr != nil && isGraphRetryable(readErr) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.GetNeighbors GetNode", readErr).Log()
		}
		if readErr == nil && node != nil {
			obj := g.nodeToObject(node)
			if obj != nil {
				objKind, _ := obj[objects.FieldKeyKind].(string)
				if err := g.checkPermission(secCtx, "read", objKind); err == nil {
					neighbors = append(neighbors, obj)
				}
			}
		}
	}

	return neighbors, nil
}
