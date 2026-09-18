package mcp

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// HandleGraphTraversal handles the graph_traversal tool call.
// Performs multi-hop graph traversal starting from a node and following relationships.
//
// Parameters:
//   - ctx: Request context for cancellation and timeout support
//   - args: Map containing required parameters:
//   - start_node_id (string): ID of the starting node (required).
//   - relationship (string): Relationship type to traverse (e.g., "HAS_CHILD", "RELATES_TO"). Optional, traverses all if not specified.
//   - direction (string): Traversal direction ("outgoing", "incoming", "both"). Defaults to "outgoing".
//   - max_depth (int): Maximum traversal depth. Defaults to 3.
//   - filter_labels ([]string): Filter nodes by labels. Optional.
//   - filter_properties (map): Filter nodes by properties. Optional.
//   - limit (int): Maximum number of nodes to return. Optional, defaults to 100.
//
// Returns:
//   - any: Map containing nodes, edges, and traversal metadata.
//   - error: Returns an error if traversal fails or graph backend is unavailable.
//
// Example:
//
//	result, err := HandleGraphTraversal(ctx, map[string]any{
//	    "start_node_id": "BLI-623",
//	    "relationship": "HAS_CHILD",
//	    "direction": "outgoing",
//	    "max_depth": 2,
//	})
func HandleGraphTraversal(ctx context.Context, args map[string]any) (any, error) {
	// Extract parameters
	startNodeID, ok := args["start_node_id"].(string)
	if !ok || startNodeID == emptyValue {
		// Use elicitation to ask for the missing parameter
		return nil, NewElicitationError(
			"Missing required parameter: start_node_id",
			[]ElicitationParam{
				ElicitParamWithExample(
					"start_node_id",
					"The ID of the starting node for graph traversal (e.g., BLI-626, GOAL-123)",
					"string",
					true,
					"BLI-626",
				),
			},
		)
	}

	relationship, _ := args["relationship"].(string)
	direction, ok := args["direction"].(string)
	if !ok || direction == emptyValue {
		direction = "outgoing"
	}

	maxDepth := DefaultGraphMaxDepth
	switch md := args["max_depth"].(type) {
	case float64:
		maxDepth = int(md)
	case int:
		maxDepth = md
	}

	limit := DefaultGraphLimit
	switch l := args["limit"].(type) {
	case float64:
		limit = int(l)
	case int:
		limit = l
	}

	filterLabels := []string{}
	if labels, ok := args["filter_labels"].([]any); ok {
		for _, label := range labels {
			if str, ok := label.(string); ok {
				filterLabels = append(filterLabels, str)
			}
		}
	}

	filterProperties := map[string]any{}
	if props, ok := args["filter_properties"].(map[string]any); ok {
		filterProperties = props
	}

	// Get graph connection manager
	graphMgr := GetGraphConnectionManager()

	// Execute traversal via graph backend
	nodes, edges, err := graphMgr.ExecuteTraversal(
		ctx,
		startNodeID,
		relationship,
		direction,
		maxDepth,
		filterLabels,
		filterProperties,
		limit,
	)

	if err != nil {
		// Graph backend not available - return informative error
		return map[string]any{
			"start_node_id":      startNodeID,
			"relationship":       relationship,
			"direction":          direction,
			"max_depth":          maxDepth,
			"nodes":              []any{},
			"edges":              []any{},
			"error":              err.Error(),
			objects.FieldKeyNote: fmt.Sprintf("Graph backend not available. Set %s=true and configure connection settings.", brand.EnvVar("GRAPH_ENABLED")),
		}, err
	}

	return map[string]any{
		"start_node_id": startNodeID,
		"relationship":  relationship,
		"direction":     direction,
		"max_depth":     maxDepth,
		"nodes":         nodes,
		"edges":         edges,
		"count":         len(nodes),
	}, nil
}

// HandleResolveReferences handles the resolve_references tool call.
// Resolves object references (e.g., "goal:GOAL-123", "milestone:MIL-456") to actual objects.
//
// Parameters:
//   - ctx: Request context for cancellation and timeout support
//   - args: Map containing required parameters:
//   - references ([]string): List of references to resolve (required).
//   - include_related (bool): Include related objects. Defaults to false.
//   - format (string): Output format ("json", "yaml"). Defaults to "json".
//
// Returns:
//   - any: Map containing resolved objects and unresolved references.
//   - error: Returns an error if resolution fails.
//
// Example:
//
//	result, err := HandleResolveReferences(ctx, map[string]any{
//	    "references": []string{"goal:GOAL-123", "milestone:MIL-456"},
//	    "include_related": true,
//	})
func HandleResolveReferences(ctx context.Context, args map[string]any) (any, error) {
	// Extract parameters
	refsInterface, ok := args["references"].([]any)
	if !ok {
		// Use elicitation to ask for the missing parameter
		return nil, NewElicitationError(
			"Missing required parameter: references",
			[]ElicitationParam{
				ElicitParamWithExample(
					"references",
					"List of object references to resolve (e.g., [\"goal:GOAL-123\", \"milestone:MIL-456\"])",
					"array",
					true,
					[]string{"goal:GOAL-123", "milestone:MIL-456"},
				),
			},
		)
	}

	references := []string{}
	for _, ref := range refsInterface {
		if str, ok := ref.(string); ok {
			references = append(references, str)
		}
	}

	if len(references) == 0 {
		// Use elicitation to ask for at least one reference
		return nil, NewElicitationError(
			"At least one reference is required",
			[]ElicitationParam{
				ElicitParamWithExample(
					"references",
					"List of object references to resolve. Format: \"kind:ID\" (e.g., \"goal:GOAL-123\")",
					"array",
					true,
					[]string{"goal:GOAL-123"},
				),
			},
		)
	}

	includeRelated, _ := args["include_related"].(bool)
	format, ok := args[objects.FieldKeyFormat].(string)
	if !ok || format == emptyValue {
		format = "json"
	}

	// Get graph connection manager
	graphMgr := GetGraphConnectionManager()

	if !graphMgr.IsEnabled() {
		return nil, errfmt.Errorf("graph backend not enabled - set %s=true and configure connection settings", brand.EnvVar("GRAPH_ENABLED"))
	}

	// Extract depth and timeout parameters (optional)
	maxDepth := DefaultGraphMaxDepth // Default: only direct relationships
	switch md := args["max_depth"].(type) {
	case float64:
		maxDepth = int(md)
	case int:
		maxDepth = md
	}

	timeoutSeconds := DefaultGraphTimeoutSeconds // Default: 5 second timeout
	switch ts := args["timeout_seconds"].(type) {
	case float64:
		timeoutSeconds = int(ts)
	case int:
		timeoutSeconds = ts
	}

	// Resolve references via graph backend with depth control and timeout
	resolved, unresolved, err := graphMgr.ResolveReferences(ctx, references, includeRelated, maxDepth, timeoutSeconds)
	if err != nil {
		return nil, errfmt.Newf("failed to resolve references").Wrap(err)
	}

	return map[string]any{
		"resolved":             resolved,
		"unresolved":           unresolved,
		objects.FieldKeyFormat: format,
		"count":                len(resolved),
		objects.FieldKeySource: "graph",
	}, nil
}

// HandleStateAwareQuery handles the state_aware_query tool call.
// Performs queries that are aware of object lifecycle states and relationships.
//
// Parameters:
//   - ctx: Request context for cancellation and timeout support
//   - args: Map containing required parameters:
//   - query_type (string): Type of query ("active_items", "blocked_items", "dependencies", "progress"). Required.
//   - filters (map): Additional filters (status, kind, date_range, etc.). Optional.
//   - include_metrics (bool): Include calculated metrics. Defaults to false.
//   - format (string): Output format ("json", "yaml", "markdown"). Defaults to "json".
//
// Returns:
//   - any: Map containing query results and metadata.
//   - error: Returns an error if query fails.
//
// Example:
//
//	result, err := HandleStateAwareQuery(ctx, map[string]any{
//	    "query_type": "blocked_items",
//	    "filters": map[string]any{
//	        "kind": "backlog_item",
//	    },
//	})
func HandleStateAwareQuery(ctx context.Context, args map[string]any) (any, error) {
	queryType, ok := args["query_type"].(string)
	if !ok || queryType == emptyValue {
		return nil, errfmt.Errorf("query_type is required")
	}

	filters := map[string]any{}
	if f, ok := args["filters"].(map[string]any); ok {
		filters = f
	}

	includeMetrics, _ := args["include_metrics"].(bool)
	format, ok := args[objects.FieldKeyFormat].(string)
	if !ok || format == emptyValue {
		format = "json"
	}

	// Get graph connection manager
	graphMgr := GetGraphConnectionManager()

	if !graphMgr.IsEnabled() {
		return nil, errfmt.Errorf("graph backend not enabled - set %s=true and configure connection settings", brand.EnvVar("GRAPH_ENABLED"))
	}

	// Execute state-aware query via graph backend
	results, metrics, err := graphMgr.QueryStateAware(ctx, queryType, filters, includeMetrics)
	if err != nil {
		return nil, errfmt.Newf("failed to execute state-aware query").Wrap(err)
	}

	response := map[string]any{
		"query_type":           queryType,
		"results":              results,
		"count":                len(results),
		objects.FieldKeyFormat: format,
		objects.FieldKeySource: "graph",
	}

	if includeMetrics && metrics != nil {
		response[objects.FieldKeyMetrics] = metrics
	}

	return response, nil
}
