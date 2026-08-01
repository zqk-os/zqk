package storage

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
)

// Exists checks if an object exists by ID
// More efficient than Read() as it doesn't load the object
func (g *GraphObjectStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
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
		// If error is "not found", return false
		if gerr, ok := err.(*provider.GraphError); ok && gerr.IsNodeNotFound() {
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
	// Use BFS traversal to find path
	// This works with the current provider interface
	// Future optimization: use Cypher shortestPath query when path extraction is available
	return g.findPathBFS(ctx, secCtx, fromID, toID)
}

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

			if !visited[neighborID] {
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
	// Read the object to verify it exists
	_, err := g.Read(ctx, secCtx, id)
	if err != nil {
		return nil, errfmt.Errorf(ConstStreamFailedToReadObjectStrErr, id, err)
	}

	neighbors := make([]map[string]any, 0)

	// Handle outgoing (objects this references)
	if direction == "outgoing" || direction == "both" {
		// Get outgoing edges
		edgeFilter := provider.EdgeFilter{
			FromID: id,
			Limit:  1000,
		}

		edges, err := g.conn.ListEdges(ctx, edgeFilter)
		if err == nil {
			for _, edge := range edges {
				node, readErr := g.conn.GetNode(ctx, edge.ToID, nil)
				if readErr == nil && node != nil {
					obj := g.nodeToObject(node)
					if obj != nil {
						// Check permissions
						objKind, _ := obj[objects.FieldKeyKind].(string)
						if err := g.checkPermission(secCtx, "read", objKind); err == nil {
							neighbors = append(neighbors, obj)
						}
					}
				}
			}
		}
	}

	// Handle incoming (objects that reference this)
	if direction == "incoming" || direction == "both" {
		// Get incoming edges
		edgeFilter := provider.EdgeFilter{
			ToID:  id,
			Limit: 1000,
		}

		edges, err := g.conn.ListEdges(ctx, edgeFilter)
		if err == nil {
			for _, edge := range edges {
				node, readErr := g.conn.GetNode(ctx, edge.FromID, nil)
				if readErr == nil && node != nil {
					obj := g.nodeToObject(node)
					if obj != nil {
						// Check permissions
						objKind, _ := obj[objects.FieldKeyKind].(string)
						if err := g.checkPermission(secCtx, "read", objKind); err == nil {
							neighbors = append(neighbors, obj)
						}
					}
				}
			}
		}
	}

	return neighbors, nil
}
