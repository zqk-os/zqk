package mcp

import (
	"context"
	"maps"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/memgraph"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// graphEnvSuffix* are passed to brand.EnvVar (avoids pattern-8 hits on ("GRAPH_*") call sites).
const (
	graphEnvSuffixDatabase = "GRAPH_DATABASE"
	graphEnvSuffixEnabled  = "GRAPH_ENABLED"
	graphEnvSuffixHost     = "GRAPH_HOST"
	graphEnvSuffixPassword = "GRAPH_PASSWORD"
	graphEnvSuffixPoolSize = "GRAPH_POOL_SIZE"
	graphEnvSuffixPort     = "GRAPH_PORT"
	graphEnvSuffixUsername = "GRAPH_USERNAME"
)

// graphMapKey* are keys for MCP graph query result / metrics maps.
const (
	graphMapKeyBlockedCount      = "blocked_count"
	graphMapKeyChildCount        = "child_count"
	graphMapKeyCompleteCount     = "complete_count"
	graphMapKeyCompletionPercent = "completion_percent"
	graphMapKeyDependsOn         = "depends_on"
	graphMapKeyInProgressCount   = "in_progress_count"
	graphMapKeyProgress          = "progress"
	graphMapKeyRequiredBy        = "required_by"
)

const (
	graphStatusComplete = "complete"
)

// GraphConnectionManager manages connections to the graph backend
type GraphConnectionManager struct {
	enabled bool
	pool    provider.ConnectionPool
	config  provider.ConnectionConfig
}

var globalGraphManager *GraphConnectionManager

func init() {
	globalGraphManager = &GraphConnectionManager{
		enabled: isGraphBackendEnabled(),
		config:  getGraphConfig(),
	}
}

// isGraphBackendEnabled checks if graph backend is enabled via environment/config
func isGraphBackendEnabled() bool {
	// Check environment variable (explicit opt-in required).
	// Prefer brand-derived env vars, but accept legacy and direct prefixes for compatibility.
	keys := []string{
		brand.EnvVar(graphEnvSuffixEnabled),
		"ZQK_GRAPH_ENABLED",
		"GRAPH_ENABLED",
		brand.EnvVar("MOCK_GRAPH"),
	}
	for _, key := range keys {
		if enabled := os.Getenv(key); enabled != emptyValue {
			if val, err := strconv.ParseBool(enabled); err == nil {
				return val
			}
		}
	}
	// Default: disabled until explicitly enabled
	return false
}

// getGraphConfig reads graph connection configuration from environment
func getGraphConfig() provider.ConnectionConfig {
	config := provider.ConnectionConfig{
		Host: getEnvOrDefaultAny([]string{
			brand.EnvVar(graphEnvSuffixHost),
			"ZQK_GRAPH_HOST",
			"GRAPH_HOST",
		}, "127.0.0.1"),
		Port: getEnvIntOrDefaultAny([]string{
			brand.EnvVar(graphEnvSuffixPort),
			"ZQK_GRAPH_PORT",
			"GRAPH_PORT",
		}, 7687),
		Username: getEnvOrDefaultAny([]string{
			brand.EnvVar(graphEnvSuffixUsername),
			"ZQK_GRAPH_USERNAME",
			"GRAPH_USERNAME",
		}, ""),
		Password: getEnvOrDefaultAny([]string{
			brand.EnvVar(graphEnvSuffixPassword),
			"ZQK_GRAPH_PASSWORD",
			"GRAPH_PASSWORD",
		}, ""),
		Database: getEnvOrDefaultAny([]string{
			brand.EnvVar(graphEnvSuffixDatabase),
			"ZQK_GRAPH_DATABASE",
			"GRAPH_DATABASE",
		}, ""),
		MaxConns: getEnvIntOrDefaultAny([]string{
			brand.EnvVar(graphEnvSuffixPoolSize),
			"ZQK_GRAPH_POOL_SIZE",
			"GRAPH_POOL_SIZE",
		}, 10),
	}
	return config
}

// getEnvOrDefaultAny returns the first environment variable value found from keys, otherwise defaultValue.
func getEnvOrDefaultAny(keys []string, defaultValue string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != emptyValue {
			return val
		}
	}
	return defaultValue
}

// getEnvIntOrDefaultAny returns the first environment variable value found from keys parsed as int, otherwise defaultValue.
func getEnvIntOrDefaultAny(keys []string, defaultValue int) int {
	for _, key := range keys {
		if val := os.Getenv(key); val != emptyValue {
			if intVal, err := strconv.Atoi(val); err == nil {
				return intVal
			}
		}
	}
	return defaultValue
}

// GetGraphConnectionManager returns the global graph connection manager
func GetGraphConnectionManager() *GraphConnectionManager {
	return globalGraphManager
}

// IsEnabled returns whether the graph backend is enabled
func (g *GraphConnectionManager) IsEnabled() bool {
	return isGraphBackendEnabled()
}

// GetPool returns or creates the connection pool (public method for storage integration)
func (g *GraphConnectionManager) GetPool(ctx context.Context) (provider.ConnectionPool, error) {
	return g.getPool(ctx)
}

// getPool returns or creates the connection pool (internal method)
func (g *GraphConnectionManager) getPool(ctx context.Context) (provider.ConnectionPool, error) {
	if g.pool != nil {
		return g.pool, nil
	}

	g.enabled = isGraphBackendEnabled()
	g.config = getGraphConfig()

	if !g.enabled {
		return nil, errfmt.Errorf("graph backend not enabled - set %s=true", brand.EnvVar(graphEnvSuffixEnabled))
	}

	// Check if mock graph is requested
	if os.Getenv(brand.EnvVar("MOCK_GRAPH")) == "true" {
		mockProvider := provider.NewMockGraphProvider()
		pool, err := mockProvider.CreatePool(ctx, g.config)
		if err != nil {
			return nil, errfmt.Newf("failed to create mock graph connection pool").Wrap(err)
		}
		g.pool = pool
		return pool, nil
	}

	// Create MemGraph provider
	mgProvider := memgraph.NewMemGraphProvider(&memgraph.MemGraphConfig{
		Host:     g.config.Host,
		Port:     g.config.Port,
		Username: g.config.Username,
		Password: g.config.Password,
		Database: g.config.Database,
		PoolSize: g.config.MaxConns,
	})

	// Create connection pool
	pool, err := mgProvider.CreatePool(ctx, g.config)
	if err != nil {
		return nil, errfmt.Newf("failed to create graph connection pool").Wrap(err)
	}

	g.pool = pool
	return pool, nil
}

// ExecuteTraversal executes a graph traversal query
func (g *GraphConnectionManager) ExecuteTraversal(ctx context.Context, startNodeID, relationship, direction string, maxDepth int, filterLabels []string, filterProperties map[string]any, limit int) (nodes, relationships []any, err error) {
	pool, err := g.getPool(ctx)
	if err != nil {
		return nil, nil, err
	}

	var edges []any

	// Convert direction string to provider.Direction
	var dir provider.Direction
	switch direction {
	case "incoming":
		dir = provider.DirectionIncoming
	case "both":
		dir = provider.DirectionBoth
	default:
		dir = provider.DirectionOutgoing
	}

	// Build traversal query
	traversal := provider.TraversalQuery{
		StartNodeID:  startNodeID,
		Relationship: relationship,
		Direction:    dir,
		MaxDepth:     maxDepth,
		Filter: provider.NodeFilter{
			Labels:     filterLabels,
			Properties: filterProperties,
		},
	}

	// Execute traversal
	err = pool.Execute(ctx, func(conn provider.GraphConnection) error {
		result, err := conn.ExecuteTraversal(ctx, traversal)
		if err != nil {
			return errfmt.Newf("traversal execution failed").Wrap(err)
		}

		// Convert nodes to maps
		for _, node := range result.Nodes {
			nodeMap := nodeToMap(node)
			nodes = append(nodes, nodeMap)
			if len(nodes) >= limit {
				break
			}
		}

		// Convert edges to maps
		for _, edge := range result.Edges {
			edgeMap := edgeToMap(edge)
			edges = append(edges, edgeMap)
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	relationships = edges
	return nodes, relationships, nil
}

// ResolveReferences resolves object references using graph backend
func (g *GraphConnectionManager) ResolveReferences(ctx context.Context, references []string, includeRelated bool, maxDepth, timeoutSeconds int) ([]any, []string, error) {
	pool, err := g.getPool(ctx)
	if err != nil {
		return nil, nil, err
	}

	resolved := []any{}
	unresolved := []string{}

	// Set defaults for depth and timeout
	if maxDepth <= 0 {
		maxDepth = 2 // Default: only direct relationships
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5 // Default: 5 second timeout
	}

	for _, ref := range references {
		// Parse reference format: "kind:id" or just "id"
		kind, id := parseReference(ref)
		if kind == emptyValue {
			// Try to infer kind from ID pattern
			kind = inferKindFromID(id)
		}

		// Query graph for node by ID
		var found bool
		err = pool.Execute(ctx, func(conn provider.GraphConnection) error {
			// Try to find node by ID
			filter := provider.NodeFilter{
				Properties: map[string]any{
					objects.FieldKeyID: id,
				},
				Limit: 1, // Only need one match
			}
			if kind != emptyValue {
				filter.Labels = []string{kind}
			}

			nodes, err := conn.ListNodes(ctx, filter)
			if err != nil {
				return err
			}

			if len(nodes) > 0 {
				found = true
				nodeMap := nodeToMap(nodes[0])
				resolved = append(resolved, nodeMap)

				// Include related objects if requested
				if includeRelated {
					// Find related nodes via relationships with depth control and timeout
					relatedNodes, err := findRelatedNodesWithDepth(ctx, conn, nodes[0].ID, maxDepth, timeoutSeconds)
					if err == nil {
						for _, relatedNode := range relatedNodes {
							relatedMap := nodeToMap(relatedNode)
							resolved = append(resolved, relatedMap)
						}
					}
				}
			}

			return nil
		})

		if err != nil {
			unresolved = append(unresolved, ref)
			continue
		}

		if !found {
			unresolved = append(unresolved, ref)
		}
	}

	return resolved, unresolved, nil
}

// QueryStateAware performs state-aware queries using graph backend
func (g *GraphConnectionManager) QueryStateAware(ctx context.Context, queryType string, filters map[string]any, includeMetrics bool) ([]any, map[string]any, error) {
	pool, err := g.getPool(ctx)
	if err != nil {
		return nil, nil, err
	}

	results := []any{}
	metrics := map[string]any{}

	switch queryType {
	case "active_items":
		// Find all active items
		err = pool.Execute(ctx, func(conn provider.GraphConnection) error {
			filter := provider.NodeFilter{
				Properties: map[string]any{
					objects.FieldKeyStatus: []any{"active", "in_progress"},
				},
			}
			// Apply additional filters if provided
			maps.Copy(filter.Properties, filters)

			nodes, err := conn.ListNodes(ctx, filter)
			if err != nil {
				return err
			}

			for _, node := range nodes {
				results = append(results, nodeToMap(node))
			}

			return nil
		})

	case "blocked_items":
		// Find items that are blocked (have unresolved dependencies or BLOCKS relationships)
		err = pool.Execute(ctx, func(conn provider.GraphConnection) error {
			// Strategy: Find items that:
			// 1. Have status="blocked"
			// 2. Have DEPENDS_ON relationships to incomplete items
			// 3. Have BLOCKS relationships pointing to them

			// First, find items with status="blocked"
			blockedFilter := provider.NodeFilter{
				Properties: map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusBlocked,
				},
			}
			// Apply additional filters if provided
			maps.Copy(blockedFilter.Properties, filters)

			blockedNodes, err := conn.ListNodes(ctx, blockedFilter)
			if err != nil {
				return err
			}

			// Add explicitly blocked items
			for _, node := range blockedNodes {
				results = append(results, nodeToMap(node))
			}

			// Find items blocked by DEPENDS_ON relationships to incomplete items
			// Get all nodes with DEPENDS_ON outgoing edges
			dependsOnFilter := provider.EdgeFilter{
				Type:  "DEPENDS_ON",
				Limit: 1000, // Reasonable limit
			}
			dependsOnEdges, err := conn.ListEdges(ctx, dependsOnFilter)
			if err == nil {
				seenBlocked := make(map[string]bool)
				for _, edge := range dependsOnEdges {
					// Check if the dependency (target) is incomplete
					targetNode, err := conn.GetNode(ctx, edge.ToID, nil)
					if err == nil && targetNode != nil {
						status, _ := targetNode.Properties[objects.FieldKeyStatus].(string)
						// Item is blocked if dependency is not complete (BLI-958: terminal statuses could come from config/lifecycle)
						if status != graphStatusComplete && status != objects.ObjectStatusArchived {
							// The source item (edge.FromID) is blocked
							if !seenBlocked[edge.FromID] {
								sourceNode, err := conn.GetNode(ctx, edge.FromID, nil)
								if err == nil && sourceNode != nil {
									results = append(results, nodeToMap(sourceNode))
									seenBlocked[edge.FromID] = true
								}
							}
						}
					}
				}
			}

			// Find items blocked by BLOCKS relationships (incoming BLOCKS edges)
			blocksFilter := provider.EdgeFilter{
				Type:  "BLOCKS",
				Limit: 1000,
			}
			blocksEdges, err := conn.ListEdges(ctx, blocksFilter)
			if err == nil {
				seenBlocked := make(map[string]bool)
				for _, edge := range blocksEdges {
					// The target of a BLOCKS edge is blocked
					if !seenBlocked[edge.ToID] {
						blockedNode, err := conn.GetNode(ctx, edge.ToID, nil)
						if err == nil && blockedNode != nil {
							// Check if already in results
							alreadyAdded := false
							for _, result := range results {
								if resultMap, ok := result.(map[string]any); ok {
									if resultMap[objects.FieldKeyID] == blockedNode.ID {
										alreadyAdded = true
										break
									}
								}
							}
							if !alreadyAdded {
								results = append(results, nodeToMap(blockedNode))
								seenBlocked[edge.ToID] = true
							}
						}
					}
				}
			}

			return nil
		})

	case "dependencies":
		// Find dependency chains using DEPENDS_ON relationships
		err = pool.Execute(ctx, func(conn provider.GraphConnection) error {
			// If a specific item ID is provided, find its dependency chain
			startID, hasStartID := filters[objects.FieldKeyID].(string)

			if hasStartID && startID != emptyValue {
				// Traverse dependency chain starting from the specified item
				traversal := provider.TraversalQuery{
					StartNodeID:  startID,
					Relationship: "DEPENDS_ON",
					Direction:    provider.DirectionOutgoing,
					MaxDepth:     10, // Reasonable depth limit
					Filter:       provider.NodeFilter{},
				}

				result, err := conn.ExecuteTraversal(ctx, traversal)
				if err == nil {
					for _, node := range result.Nodes {
						results = append(results, nodeToMap(node))
					}
				}
			} else {
				// Find all items with dependencies (have DEPENDS_ON outgoing edges)
				dependsOnFilter := provider.EdgeFilter{
					Type:  "DEPENDS_ON",
					Limit: 1000,
				}
				dependsOnEdges, err := conn.ListEdges(ctx, dependsOnFilter)
				if err == nil {
					seen := make(map[string]bool)
					for _, edge := range dependsOnEdges {
						// Include both source and target in results
						if !seen[edge.FromID] {
							sourceNode, err := conn.GetNode(ctx, edge.FromID, nil)
							if err == nil && sourceNode != nil {
								nodeMap := nodeToMap(sourceNode)
								// Add dependency info
								nodeMap[graphMapKeyDependsOn] = edge.ToID
								results = append(results, nodeMap)
								seen[edge.FromID] = true
							}
						}
						if !seen[edge.ToID] {
							targetNode, err := conn.GetNode(ctx, edge.ToID, nil)
							if err == nil && targetNode != nil {
								nodeMap := nodeToMap(targetNode)
								// Add reverse dependency info
								nodeMap[graphMapKeyRequiredBy] = edge.FromID
								results = append(results, nodeMap)
								seen[edge.ToID] = true
							}
						}
					}
				}
			}

			return nil
		})

	case "progress":
		// Calculate progress metrics across hierarchies
		err = pool.Execute(ctx, func(conn provider.GraphConnection) error {
			// If a specific item ID is provided, calculate progress for that item and its children
			startID, hasStartID := filters[objects.FieldKeyID].(string)

			if hasStartID && startID != emptyValue {
				progress, err := calculateProgressForNode(ctx, conn, startID)
				if err == nil {
					results = append(results, progress)
					if includeMetrics {
						metrics[graphMapKeyProgress] = progress
					}
				}
			} else {
				// Calculate system-wide progress metrics
				// Count items by status across all object types
				statusCounts := make(map[string]int)
				totalCount := 0

				// Get all nodes (no filter)
				allNodesFilter := provider.NodeFilter{
					Limit: 10000, // Reasonable limit for progress calculation
				}
				allNodes, err := conn.ListNodes(ctx, allNodesFilter)
				if err == nil {
					for _, node := range allNodes {
						totalCount++
						if status, ok := node.Properties[objects.FieldKeyStatus].(string); ok {
							statusCounts[status]++
						}
					}
				}

				// Calculate completion percentage
				completeCount := statusCounts[graphStatusComplete] + statusCounts[objects.ObjectStatusArchived]
				completionPercent := 0.0
				if totalCount > 0 {
					completionPercent = float64(completeCount) / float64(totalCount) * 100.0
				}

				progressData := map[string]any{
					"total_items":        totalCount,
					"status_breakdown":   statusCounts,
					"complete_count":     completeCount,
					"completion_percent": completionPercent,
				}

				results = append(results, progressData)
				if includeMetrics {
					metrics[graphMapKeyProgress] = progressData
				}
			}

			return nil
		})

	default:
		return nil, nil, errfmt.Errorf("unknown query_type: %s", queryType)
	}

	if err != nil {
		return nil, nil, err
	}

	return results, metrics, nil
}

// findRelatedNodesWithDepth finds nodes connected to the given node via relationships
// with depth control and timeout protection to prevent cyclic loops
func findRelatedNodesWithDepth(ctx context.Context, conn provider.GraphConnection, nodeID string, maxDepth int, timeoutSeconds int) ([]*provider.Node, error) {
	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Track visited nodes to prevent cycles
	visited := make(map[string]bool)
	relatedNodes := []*provider.Node{}

	// Use a queue for breadth-first traversal with depth tracking
	type nodeWithDepth struct {
		node  *provider.Node
		depth int
	}
	queue := []nodeWithDepth{}

	// Start with the initial node
	startNode, err := conn.GetNode(timeoutCtx, nodeID, nil)
	if err != nil {
		return nil, err
	}
	if startNode == nil {
		return nil, errfmt.Errorf("node %s not found", nodeID)
	}

	queue = append(queue, nodeWithDepth{node: startNode, depth: 0})
	visited[nodeID] = true

	// Process queue with depth limit
	for len(queue) > 0 {
		// Check timeout
		select {
		case <-timeoutCtx.Done():
			return relatedNodes, errfmt.Errorf("timeout after %d seconds", timeoutSeconds)
		default:
		}

		current := queue[0]
		queue = queue[1:]

		// Skip if we've reached max depth
		if current.depth >= maxDepth {
			continue
		}

		// Find outgoing relationships
		outgoingFilter := provider.EdgeFilter{
			FromID: current.node.ID,
			Limit:  100, // Reasonable limit for related nodes
		}
		outgoingEdges, err := conn.ListEdges(timeoutCtx, outgoingFilter)
		if err == nil {
			for _, edge := range outgoingEdges {
				// Skip if already visited
				if visited[edge.ToID] {
					continue
				}

				// Get the target node
				if targetNode, err := conn.GetNode(timeoutCtx, edge.ToID, nil); err == nil && targetNode != nil {
					visited[edge.ToID] = true
					relatedNodes = append(relatedNodes, targetNode)

					// Add to queue for next depth level
					if current.depth+1 < maxDepth {
						queue = append(queue, nodeWithDepth{node: targetNode, depth: current.depth + 1})
					}
				}
			}
		}

		// Find incoming relationships
		incomingFilter := provider.EdgeFilter{
			ToID:  current.node.ID,
			Limit: 100,
		}
		incomingEdges, err := conn.ListEdges(timeoutCtx, incomingFilter)
		if err == nil {
			for _, edge := range incomingEdges {
				// Skip if already visited
				if visited[edge.FromID] {
					continue
				}

				// Get the source node
				if sourceNode, err := conn.GetNode(timeoutCtx, edge.FromID, nil); err == nil && sourceNode != nil {
					visited[edge.FromID] = true
					relatedNodes = append(relatedNodes, sourceNode)

					// Add to queue for next depth level
					if current.depth+1 < maxDepth {
						queue = append(queue, nodeWithDepth{node: sourceNode, depth: current.depth + 1})
					}
				}
			}
		}
	}

	return relatedNodes, nil
}

// calculateProgressForNode calculates progress for a node and its children
func calculateProgressForNode(ctx context.Context, conn provider.GraphConnection, nodeID string) (map[string]any, error) {
	// Get the node itself
	node, err := conn.GetNode(ctx, nodeID, nil)
	if err != nil {
		return nil, err
	}

	progress := map[string]any{
		objects.FieldKeyID:     nodeID,
		objects.FieldKeyStatus: node.Properties[objects.FieldKeyStatus],
	}

	// Find child nodes via common relationship types
	childRelationships := []string{"BELONGS_TO", "IMPLEMENTS", "CONTAINS", "ORGANIZES", "SUPPORTS"}
	childCount := 0
	completeCount := 0
	inProgressCount := 0
	blockedCount := 0

	for _, relType := range childRelationships {
		// Find outgoing edges (this node contains/organizes children)
		edgeFilter := provider.EdgeFilter{
			FromID: nodeID,
			Type:   relType,
			Limit:  1000,
		}
		edges, err := conn.ListEdges(ctx, edgeFilter)
		if err == nil {
			for _, edge := range edges {
				childNode, err := conn.GetNode(ctx, edge.ToID, nil)
				if err == nil && childNode != nil {
					childCount++
					if status, ok := childNode.Properties[objects.FieldKeyStatus].(string); ok {
						switch status {
						case graphStatusComplete, objects.ObjectStatusArchived:
							completeCount++
						case objects.ObjectStatusInProgress, objects.ObjectStatusActive:
							inProgressCount++
						case objects.ObjectStatusBlocked:
							blockedCount++
						}
					}
				}
			}
		}

		// Also check incoming edges (children belong to this node)
		edgeFilter = provider.EdgeFilter{
			ToID:  nodeID,
			Type:  relType,
			Limit: 1000,
		}
		edges, err = conn.ListEdges(ctx, edgeFilter)
		if err == nil {
			for _, edge := range edges {
				childNode, err := conn.GetNode(ctx, edge.FromID, nil)
				if err == nil && childNode != nil {
					childCount++
					if status, ok := childNode.Properties[objects.FieldKeyStatus].(string); ok {
						switch status {
						case graphStatusComplete, objects.ObjectStatusArchived:
							completeCount++
						case objects.ObjectStatusInProgress, objects.ObjectStatusActive:
							inProgressCount++
						case objects.ObjectStatusBlocked:
							blockedCount++
						}
					}
				}
			}
		}
	}

	progress[graphMapKeyChildCount] = childCount
	progress[graphMapKeyCompleteCount] = completeCount
	progress[graphMapKeyInProgressCount] = inProgressCount
	progress[graphMapKeyBlockedCount] = blockedCount

	if childCount > 0 {
		completionPercent := float64(completeCount) / float64(childCount) * 100.0
		progress[graphMapKeyCompletionPercent] = completionPercent
	} else {
		progress[graphMapKeyCompletionPercent] = 0.0
	}

	return progress, nil
}

// nodeToMap converts a graph node to a map
func nodeToMap(node *provider.Node) map[string]any {
	result := map[string]any{
		objects.FieldKeyID: node.ID,
		"labels":           node.Labels,
	}
	maps.Copy(result, node.Properties)
	return result
}

// edgeToMap converts a graph edge to a map
func edgeToMap(edge *provider.Edge) map[string]any {
	return map[string]any{
		objects.FieldKeyType: edge.Type,
		"from_id":            edge.FromID,
		"to_id":              edge.ToID,
		"properties":         edge.Properties,
	}
}

// parseReference parses a reference string into kind and id
// parseReference is a convenience wrapper around ParseReference.
// Kept for backward compatibility with existing code.
func parseReference(ref string) (kind, id string) {
	return ParseReference(ref)
}

// inferKindFromID infers the object kind from an ID pattern
func inferKindFromID(id string) string {
	if strings.HasPrefix(id, "BLI-") {
		return objects.KindBacklogItem
	}
	if strings.HasPrefix(id, "MIL-") {
		return objects.KindMilestone
	}
	if strings.HasPrefix(id, "GOAL-") {
		return objects.KindGoal
	}
	if strings.HasPrefix(id, "WS-") {
		return objects.KindWorkstream
	}
	if strings.HasPrefix(id, "PRI-") || strings.HasPrefix(id, "PRIO-") {
		return objects.KindPriorityPlan
	}
	if strings.HasPrefix(id, "REQ-") || strings.HasPrefix(id, "REQU-") {
		return objects.KindRequirement
	}
	if strings.HasPrefix(id, "TEST-") {
		return objects.KindTestCase
	}
	if strings.HasPrefix(id, "CRIT-") {
		return objects.KindCriteria
	}
	if strings.HasPrefix(id, "DEC-") {
		return objects.KindDecision
	}
	if strings.HasPrefix(id, "ROAD-") {
		return objects.KindRoadmap
	}
	return ""
}
