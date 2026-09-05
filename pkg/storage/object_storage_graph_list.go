package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// List lists objects with filtering, sorting, pagination, and grouping
// List retrieves objects matching the filter criteria
//
//nolint:gocyclo // Function orchestrates query building and result processing; complexity reduced via helper methods
func (g *GraphObjectStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) { //nolint:gocritic // Interface requires value semantics for ListFilter
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := g.checkPermission(secCtx, "read", filter.Kind); err != nil {
		return nil, err
	}

	// Build Cypher query
	query, params := g.buildListQuery(&filter, storageCtx, secCtx)

	// Execute query
	queryResult, err := g.executeListQuery(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Convert results to objects
	objectList := g.convertQueryResultsToObjects(queryResult)

	// Build and return result
	return g.buildListResult(objectList, &filter, storageCtx), nil
}

// buildListQuery builds the complete Cypher query for listing objects
func (g *GraphObjectStorage) buildListQuery(filter *ListFilter, storageCtx *pkgctx.StorageContext, secCtx *pkgctx.SecurityContext) (query string, params map[string]any) {
	label := toLabel(filter.Kind)
	query = fmt.Sprintf(ConstStreamMatchNStrEntity, label)
	params = make(map[string]any)

	// Inject namespace_id if restricted by security context
	if secCtx != nil && secCtx.NamespaceID != "" && secCtx.NamespaceID != "*" {
		if filter.Filters == nil {
			filter.Filters = make(map[string]any)
		}
		filter.Filters[objects.FieldKeyNamespaceID] = secCtx.NamespaceID
	}

	// Add filters
	query = g.addFiltersToQuery(query, filter, params)

	// Calculate effective limit
	effectiveLimit := g.calculateEffectiveLimit(filter, storageCtx)

	// Add grouping, return, and sorting
	query = g.addGroupingAndReturnToQuery(query, filter, storageCtx)

	// Add pagination
	query = g.addPaginationToQuery(query, filter, effectiveLimit)

	return query, params
}

// addFiltersToQuery adds WHERE conditions to the query
func (g *GraphObjectStorage) addFiltersToQuery(query string, filter *ListFilter, params map[string]any) string {
	if len(filter.Filters) == 0 {
		return query
	}

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
			fieldExpr := fmt.Sprintf("n.%s", field)
			if strings.HasSuffix(field, "_at") {
				fieldExpr = fmt.Sprintf("toString(n.%s)", field)
			}
			conditions = append(conditions, fmt.Sprintf("%s = $%s", fieldExpr, paramName))
			params[paramName] = filterValue
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	return query
}

// calculateEffectiveLimit returns the effective list limit.
// When filter.Limit is 0, return 0. When filter.Limit > 0 (explicit), return it unchanged;
// MaxPageSize is not applied to explicit limits.
func (g *GraphObjectStorage) calculateEffectiveLimit(filter *ListFilter, _ *pkgctx.StorageContext) int {
	if filter == nil {
		return 0
	}
	if filter.Limit == 0 {
		return 0
	}
	return filter.Limit
}

// addGroupingAndReturnToQuery adds WITH, RETURN, and ORDER BY clauses
func (g *GraphObjectStorage) addGroupingAndReturnToQuery(query string, filter *ListFilter, storageCtx *pkgctx.StorageContext) string {
	if filter.GroupBy != emptyValue && storageCtx.EnableGrouping {
		groupKeyExpr := fmt.Sprintf("n.%s", filter.GroupBy)
		if filter.GroupBy == "created_at" || filter.GroupBy == "updated_at" {
			groupKeyExpr = fmt.Sprintf("toString(n.%s)", filter.GroupBy)
		}
		query += fmt.Sprintf(" WITH n, %s AS groupKey", groupKeyExpr)
		query += ConstStreamReturnNGroupkey
		query += ConstStreamOrderByGroupkey
		if filter.SortBy != emptyValue {
			order := g.getSortOrder(filter.SortAsc)
			sortExpr := fmt.Sprintf("n.%s", filter.SortBy)
			if filter.SortBy == "created_at" || filter.SortBy == "updated_at" {
				sortExpr = fmt.Sprintf("toString(n.%s)", filter.SortBy)
			}
			query += fmt.Sprintf(", %s %s", sortExpr, order)
		}
	} else {
		query += " RETURN n"
		if filter.SortBy != emptyValue {
			order := g.getSortOrder(filter.SortAsc)
			sortExpr := fmt.Sprintf("n.%s", filter.SortBy)
			if filter.SortBy == "created_at" || filter.SortBy == "updated_at" {
				sortExpr = fmt.Sprintf("toString(n.%s)", filter.SortBy)
			}
			query += fmt.Sprintf(" ORDER BY %s %s", sortExpr, order)
		}
	}
	return query
}

// getSortOrder returns "ASC" or "DESC" based on sort direction
func (g *GraphObjectStorage) getSortOrder(sortAsc bool) string {
	if sortAsc {
		return "ASC"
	}
	return "DESC"
}

// addPaginationToQuery adds SKIP and LIMIT clauses
func (g *GraphObjectStorage) addPaginationToQuery(query string, filter *ListFilter, effectiveLimit int) string {
	if effectiveLimit > 0 {
		if filter.Offset > 0 {
			query += fmt.Sprintf(" SKIP %d", filter.Offset)
		}
		query += fmt.Sprintf(" LIMIT %d", effectiveLimit)
	}
	return query
}

// executeListQuery executes the Cypher query
func (g *GraphObjectStorage) executeListQuery(ctx context.Context, query string, params map[string]any) (*provider.QueryResult, error) {
	cypherQuery := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    query,
		Params:   params,
	}

	queryResult, err := g.conn.ExecuteQuery(ctx, cypherQuery)
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.executeListQuery", err).Log()
		}
		return nil, errfmt.Newf(ConstStreamFailedToExecuteQuery).Wrap(err)
	}

	return queryResult, nil
}

// convertQueryResultsToObjects converts query results to object maps
func (g *GraphObjectStorage) convertQueryResultsToObjects(queryResult *provider.QueryResult) []map[string]any {
	var objectList []map[string]any

	// QueryResult has Nodes, Edges, and Rows
	// ExecuteQuery now converts neo4j.Node to provider.Node and populates both
	if len(queryResult.Nodes) > 0 {
		for _, node := range queryResult.Nodes {
			if node != nil {
				if obj := g.nodeToObject(node); obj != nil {
					objectList = append(objectList, obj)
				}
			}
		}
	}

	// Also check rows as fallback (in case Nodes wasn't populated)
	if len(objectList) == 0 && len(queryResult.Rows) > 0 {
		objectList = g.extractObjectsFromRows(queryResult.Rows)
	}

	return objectList
}

// extractObjectsFromRows extracts objects from query result rows
func (g *GraphObjectStorage) extractObjectsFromRows(rows []map[string]any) []map[string]any {
	var objectList []map[string]any

	for _, row := range rows {
		node := g.extractNodeFromRow(row)
		if node != nil {
			if obj := g.nodeToObject(node); obj != nil {
				objectList = append(objectList, obj)
			}
		}
	}

	return objectList
}

// extractNodeFromRow extracts a node from a query result row
func (g *GraphObjectStorage) extractNodeFromRow(row map[string]any) *provider.Node {
	// First try: row["n"] as *provider.Node (already converted by ExecuteQuery)
	if nodeValue, ok := row["n"].(*provider.Node); ok {
		return nodeValue
	}

	// Second try: iterate through row values to find a node
	for _, rowValue := range row {
		if nodeValue, ok := rowValue.(*provider.Node); ok {
			return nodeValue
		}
	}

	return nil
}

// buildListResult builds the final QueryResult with grouping if needed
func (g *GraphObjectStorage) buildListResult(objectList []map[string]any, filter *ListFilter, storageCtx *pkgctx.StorageContext) *QueryResult {
	result := &QueryResult{
		Groups: make(map[string][]map[string]any),
		Meta:   map[string]any{"total_count": len(objectList)},
	}

	// Apply grouping if requested (post-processing for file-like behavior); keep full maps until groupBy runs.
	if filter != nil && filter.GroupBy != emptyValue && storageCtx != nil && storageCtx.EnableGrouping {
		grouped := g.groupObjects(objectList, filter.GroupBy, storageCtx.MaxGroupSize)
		result.Groups = grouped
		result.Meta["total_groups"] = len(grouped)
		objectList = g.flattenGroups(grouped)
	}

	if filter != nil {
		objectList = applyListFieldProjection(filter.Kind, *filter, objectList)
	}
	result.Objects = objectList
	if filter != nil {
		projectQueryResultGroups(filter.Kind, *filter, result)
	}
	result.Meta[ConstStreamReturnedCount] = len(objectList)
	return result
}

// groupObjects groups objects by a field or comma-separated fields
func (g *GraphObjectStorage) groupObjects(objList []map[string]any, groupBy string, maxGroups int) map[string][]map[string]any {
	groups := make(map[string][]map[string]any)

	groupFields := strings.Split(groupBy, ",")
	for i := range groupFields {
		groupFields[i] = strings.TrimSpace(groupFields[i])
	}

	for _, obj := range objList {
		var groupValueParts []string
		for _, field := range groupFields {
			if val, ok := obj[field]; ok && val != nil && fmt.Sprintf("%v", val) != "" {
				groupValueParts = append(groupValueParts, fmt.Sprintf("%v", val))
			} else {
				groupValueParts = append(groupValueParts, "") // Empty/null values grouped together
			}
		}
		groupValue := strings.Join(groupValueParts, ", ")

		groups[groupValue] = append(groups[groupValue], obj)
	}

	// Limit number of groups if specified
	if maxGroups > 0 && len(groups) > maxGroups {
		// Keep only the first maxGroups groups (sorted by key)
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		limited := make(map[string][]map[string]any)
		for i := 0; i < maxGroups && i < len(keys); i++ {
			limited[keys[i]] = groups[keys[i]]
		}
		return limited
	}

	return groups
}

// flattenGroups converts grouped objects back to a flat list
func (g *GraphObjectStorage) flattenGroups(groups map[string][]map[string]any) []map[string]any {
	var result []map[string]any
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		result = append(result, groups[key]...)
	}
	return result
}

// Query executes a custom query (Cypher or Vector for graph backend)
func (g *GraphObjectStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	if query.Type != QueryTypeCypher && query.Type != QueryTypeVector && query.Type != QueryTypeHierarchicalVector {
		return nil, errfmt.Errorf(ConstStreamGraphBackendOnlySupportsCypherAndVectorQueries)
	}

	var result *provider.QueryResult
	var err error

	if query.Type == QueryTypeVector || query.Type == QueryTypeHierarchicalVector {
		vector, ok := query.Parameters["vector"].([]float32)
		if !ok {
			return nil, errfmt.Errorf(ConstStreamVectorQueryRequiresAVectorParameterOfTypeFloat32)
		}
		threshold, _ := query.Parameters["threshold"].(float32)
		if threshold == 0 {
			// fallback to float64 if provided
			if f64, ok := query.Parameters["threshold"].(float64); ok {
				threshold = float32(f64)
			} else {
				threshold = 0.8 // default threshold
			}
		}
		limit, _ := query.Parameters["limit"].(int)
		if limit == 0 {
			limit = 10
		}

		vq := provider.VectorQuery{
			Query:     query.Expression,
			Vector:    vector,
			Threshold: threshold,
			Limit:     limit,
		}
		result, err = g.conn.ExecuteVectorQuery(ctx, vq)
	} else {
		// Execute Cypher query
		cypherQuery := provider.Query{
			Language: provider.QueryLanguageCypher,
			Query:    query.Expression,
			Params:   query.Parameters,
		}
		result, err = g.conn.ExecuteQuery(ctx, cypherQuery)
	}

	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.Query ExecuteQuery", err).Log()
		}
		return nil, errfmt.Newf(ConstStreamFailedToExecuteQuery).Wrap(err)
	}

	// Convert results to objects
	var objectList []map[string]any
	// QueryResult has Nodes, Edges, and Rows
	if len(result.Nodes) > 0 {
		for _, node := range result.Nodes {
			obj := g.nodeToObject(node)
			if obj != nil {
				objectList = append(objectList, obj)
			}
		}
	}

	// Also process rows - custom Cypher queries may return non-node data
	// For aggregation queries, rows contain the aggregated results
	if len(result.Rows) > 0 {
		for _, row := range result.Rows {
			// If row contains a node, convert it
			var hasNode bool
			for _, rowValue := range row {
				if node, ok := rowValue.(*provider.Node); ok {
					obj := g.nodeToObject(node)
					if obj != nil {
						objectList = append(objectList, obj)
						hasNode = true
						break // Only take first node from each row
					}
				}
			}

			// If no node found, treat the entire row as an object (for aggregation results)
			if !hasNode {
				// Convert row to object map
				rowObj := make(map[string]any)
				for key, value := range row {
					rowObj[key] = value
				}
				if len(rowObj) > 0 {
					objectList = append(objectList, rowObj)
				}
			}
		}
	}

	return &QueryResult{
		Objects: objectList,
		Meta:    make(map[string]any),
	}, nil
}
