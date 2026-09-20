package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// SemanticTraverser provides declarative, functional graph traversal with alias/synonym resolution
// This enables fluid queries like "find commands for tasks" where "tasks" is an alias for "backlog_item"
type SemanticTraverser struct {
	conn          provider.GraphConnection
	aliasRegistry *AliasRegistry
}

// NewSemanticTraverser creates a new semantic traverser
func NewSemanticTraverser(conn provider.GraphConnection) *SemanticTraverser {
	return &SemanticTraverser{
		conn:          conn,
		aliasRegistry: NewAliasRegistry(),
	}
}

// WithAliasRegistry sets a custom alias registry
func (st *SemanticTraverser) WithAliasRegistry(registry *AliasRegistry) *SemanticTraverser {
	st.aliasRegistry = registry
	return st
}

// FindCommands finds commands using semantic, declarative queries
// Examples:
//   - FindCommands(ctx, "commands for tasks") - uses alias resolution
//   - FindCommands(ctx, "delete operations") - uses operation type resolution
//   - FindCommands(ctx, "commands that work with backlog items") - natural language
func (st *SemanticTraverser) FindCommands(ctx context.Context, query string) ([]*CommandSpecInfo, error) {
	if st.conn == nil {
		return nil, errfmt.Errorf("graph connection not provided")
	}

	// Parse query into semantic components
	parsed := st.parseQuery(query)

	// Resolve aliases and synonyms
	resolved := st.resolveSemantics(parsed)

	// Build declarative query
	cypherQuery := st.buildDeclarativeQuery(resolved)

	// Execute query
	result, err := st.conn.ExecuteQuery(ctx, provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    cypherQuery.Query,
		Params:   cypherQuery.Params,
	})
	if err != nil {
		return nil, errfmt.Newf("failed to execute semantic query").Wrap(err)
	}

	// Convert results
	return st.convertResults(result), nil
}

// TraverseCommands performs progressive traversal with semantic resolution
// This is more functional and declarative than imperative queries
// Example: TraverseCommands(ctx).For("tasks").WithOperation("delete").Execute()
func (st *SemanticTraverser) TraverseCommands(ctx context.Context) *CommandTraversal {
	return NewCommandTraversal(ctx, st.conn, st.aliasRegistry)
}

// parseQuery parses a natural language query into semantic components
func (st *SemanticTraverser) parseQuery(query string) *SemanticQuery {
	lower := strings.ToLower(query)
	parsed := &SemanticQuery{
		Original: query,
		Terms:    strings.Fields(lower),
	}

	// Extract operation types
	operationKeywords := map[string]string{
		"create": "create",
		"add":    "create",
		"new":    "create",
		"read":   "read",
		"get":    "read",
		"show":   "read",
		"list":   "list",
		"find":   "list",
		"search": "list",
		"update": "update",
		"edit":   "update",
		"modify": "update",
		"change": "update",
		"delete": "delete",
		"remove": "delete",
		"del":    "delete",
	}

	for term, opType := range operationKeywords {
		if strings.Contains(lower, term) {
			parsed.OperationType = opType
			break
		}
	}

	// Extract target kind hints
	// Look for patterns like "for X", "with X", "on X"
	for i, term := range parsed.Terms {
		if term == "for" || term == "with" || term == "on" {
			if i+1 < len(parsed.Terms) {
				parsed.TargetKindHint = parsed.Terms[i+1]
			}
		}
	}

	return parsed
}

// resolveSemantics resolves aliases and synonyms in the parsed query
func (st *SemanticTraverser) resolveSemantics(parsed *SemanticQuery) *ResolvedQuery {
	resolved := &ResolvedQuery{
		Original: parsed,
	}

	// Resolve target kind aliases
	if parsed.TargetKindHint != emptyValue {
		resolved.TargetKinds = st.aliasRegistry.Resolve(parsed.TargetKindHint)
		if len(resolved.TargetKinds) == 0 {
			// If no alias found, use the hint directly (might be a direct kind name)
			resolved.TargetKinds = []string{parsed.TargetKindHint}
		}
	}

	// Resolve operation type (already normalized in parseQuery)
	resolved.OperationType = parsed.OperationType

	return resolved
}

// buildDeclarativeQuery builds a Cypher query from resolved semantics
func (st *SemanticTraverser) buildDeclarativeQuery(resolved *ResolvedQuery) *DeclarativeQuery {
	query := &DeclarativeQuery{
		Query:  "MATCH (cmd:CommandSpec)",
		Params: make(map[string]any),
	}

	whereClauses := []string{}

	// Add target kind filters (supports multiple resolved kinds)
	if len(resolved.TargetKinds) > 0 {
		if len(resolved.TargetKinds) == 1 {
			whereClauses = append(whereClauses, "cmd.target_kind = $targetKind")
			query.Params["targetKind"] = resolved.TargetKinds[0]
		} else {
			// Multiple target kinds (OR condition)
			kindParams := []string{}
			for i, kind := range resolved.TargetKinds {
				paramName := fmt.Sprintf("targetKind%d", i)
				kindParams = append(kindParams, "$"+paramName)
				query.Params[paramName] = kind
			}
			whereClauses = append(whereClauses, fmt.Sprintf("cmd.target_kind IN [%s]", strings.Join(kindParams, ", ")))
		}
	}

	// Add operation type filter
	if resolved.OperationType != emptyValue {
		whereClauses = append(whereClauses, "cmd.operation_type = $operationType")
		query.Params["operationType"] = resolved.OperationType
	}

	// Add WHERE clause if we have filters
	if len(whereClauses) > 0 {
		query.Query += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Return command with relationships for richer traversal
	query.Query += " RETURN cmd, [(cmd)-[r]->(related) | {type: type(r), target: related}] as relationships"

	return query
}

// convertResults converts query results to CommandSpecInfo
func (st *SemanticTraverser) convertResults(result *provider.QueryResult) []*CommandSpecInfo {
	specs := []*CommandSpecInfo{}

	if result == nil {
		return specs
	}

	// Use Nodes if available
	if len(result.Nodes) > 0 {
		for _, node := range result.Nodes {
			info := st.nodeToCommandSpecInfo(node)
			if info != nil {
				specs = append(specs, info)
			}
		}
	} else {
		// Fallback to Rows
		for _, row := range result.Rows {
			if cmdValue, ok := row["cmd"]; ok {
				if node, ok := cmdValue.(*provider.Node); ok {
					info := st.nodeToCommandSpecInfo(node)
					if info != nil {
						specs = append(specs, info)
					}
				}
			}
		}
	}

	return specs
}

// nodeToCommandSpecInfo converts a graph node to CommandSpecInfo
func (st *SemanticTraverser) nodeToCommandSpecInfo(node *provider.Node) *CommandSpecInfo {
	if node == nil {
		return nil
	}

	info := &CommandSpecInfo{}
	if name, ok := node.Properties[objects.FieldKeyName].(string); ok {
		info.Name = name
	}
	if typ, ok := node.Properties[objects.FieldKeyType].(string); ok {
		info.Type = typ
	}
	if opType, ok := node.Properties["operation_type"].(string); ok {
		info.OperationType = opType
	}
	if targetKind, ok := node.Properties[objects.FieldKeyTargetKind].(string); ok {
		info.TargetKind = targetKind
	}
	if filePath, ok := node.Properties[objects.FieldKeyFilePath].(string); ok {
		info.FilePath = filePath
	}

	return info
}

// SemanticQuery represents a parsed natural language query
type SemanticQuery struct {
	Original       string
	Terms          []string
	OperationType  string
	TargetKindHint string
}

// ResolvedQuery represents a query with resolved aliases and synonyms
type ResolvedQuery struct {
	Original      *SemanticQuery
	TargetKinds   []string // Resolved target kinds (may be multiple from synonyms)
	OperationType string
}

// DeclarativeQuery represents a Cypher query with parameters
type DeclarativeQuery struct {
	Query  string
	Params map[string]any
}

// CommandTraversal provides a fluent, functional API for progressive graph traversal
type CommandTraversal struct {
	ctx           context.Context
	conn          provider.GraphConnection
	aliasRegistry *AliasRegistry
	filters       []TraversalFilter
}

// NewCommandTraversal creates a new command traversal builder
func NewCommandTraversal(ctx context.Context, conn provider.GraphConnection, registry *AliasRegistry) *CommandTraversal {
	return &CommandTraversal{
		ctx:           ctx,
		conn:          conn,
		aliasRegistry: registry,
		filters:       []TraversalFilter{},
	}
}

// For sets the target kind filter (with alias resolution)
// Example: For("tasks") resolves to ["backlog_item"] if "tasks" is an alias
func (ct *CommandTraversal) For(targetKindOrAlias string) *CommandTraversal {
	resolved := ct.aliasRegistry.Resolve(targetKindOrAlias)
	if len(resolved) == 0 {
		resolved = []string{targetKindOrAlias} // Use directly if no alias
	}
	ct.filters = append(ct.filters, TraversalFilter{
		Type:        FilterTypeTargetKind,
		TargetKinds: resolved,
	})
	return ct
}

// WithOperation sets the operation type filter
func (ct *CommandTraversal) WithOperation(opType string) *CommandTraversal {
	ct.filters = append(ct.filters, TraversalFilter{
		Type:          FilterTypeOperation,
		OperationType: opType,
	})
	return ct
}

// WithName sets the name filter (supports partial matching)
func (ct *CommandTraversal) WithName(namePattern string) *CommandTraversal {
	ct.filters = append(ct.filters, TraversalFilter{
		Type:        FilterTypeName,
		NamePattern: namePattern,
	})
	return ct
}

// Execute executes the traversal and returns matching commands
func (ct *CommandTraversal) Execute() ([]*CommandSpecInfo, error) {
	if ct.conn == nil {
		return nil, errfmt.Errorf("graph connection not provided")
	}

	// Build query from filters
	query := ct.buildQuery()

	// Execute
	result, err := ct.conn.ExecuteQuery(ct.ctx, provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    query.Query,
		Params:   query.Params,
	})
	if err != nil {
		return nil, errfmt.Newf("traversal execution failed").Wrap(err)
	}

	// Convert results
	return ct.convertResults(result), nil
}

// buildQuery builds a Cypher query from traversal filters
func (ct *CommandTraversal) buildQuery() *DeclarativeQuery {
	query := &DeclarativeQuery{
		Query:  "MATCH (cmd:CommandSpec)",
		Params: make(map[string]any),
	}

	whereClauses := []string{}

	for i, filter := range ct.filters {
		switch filter.Type {
		case FilterTypeTargetKind:
			if len(filter.TargetKinds) > 0 {
				if len(filter.TargetKinds) == 1 {
					whereClauses = append(whereClauses, "cmd.target_kind = $targetKind")
					query.Params["targetKind"] = filter.TargetKinds[0]
				} else {
					paramNames := []string{}
					for j, kind := range filter.TargetKinds {
						paramName := fmt.Sprintf("targetKind%d_%d", i, j)
						paramNames = append(paramNames, "$"+paramName)
						query.Params[paramName] = kind
					}
					whereClauses = append(whereClauses, fmt.Sprintf("cmd.target_kind IN [%s]", strings.Join(paramNames, ", ")))
				}
			}

		case FilterTypeOperation:
			if filter.OperationType != emptyValue {
				whereClauses = append(whereClauses, "cmd.operation_type = $operationType")
				query.Params["operationType"] = filter.OperationType
			}

		case FilterTypeName:
			if filter.NamePattern != emptyValue {
				whereClauses = append(whereClauses, "cmd.name CONTAINS $namePattern")
				query.Params["namePattern"] = filter.NamePattern
			}
		}
	}

	if len(whereClauses) > 0 {
		query.Query += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	query.Query += " RETURN cmd"

	return query
}

// convertResults converts query results
func (ct *CommandTraversal) convertResults(result *provider.QueryResult) []*CommandSpecInfo {
	specs := []*CommandSpecInfo{}

	if result == nil {
		return specs
	}

	if len(result.Nodes) > 0 {
		for _, node := range result.Nodes {
			info := ct.nodeToCommandSpecInfo(node)
			if info != nil {
				specs = append(specs, info)
			}
		}
	} else {
		for _, row := range result.Rows {
			if cmdValue, ok := row["cmd"]; ok {
				if node, ok := cmdValue.(*provider.Node); ok {
					info := ct.nodeToCommandSpecInfo(node)
					if info != nil {
						specs = append(specs, info)
					}
				}
			}
		}
	}

	return specs
}

// nodeToCommandSpecInfo converts a node to CommandSpecInfo
func (ct *CommandTraversal) nodeToCommandSpecInfo(node *provider.Node) *CommandSpecInfo {
	if node == nil {
		return nil
	}

	info := &CommandSpecInfo{}
	if name, ok := node.Properties[objects.FieldKeyName].(string); ok {
		info.Name = name
	}
	if typ, ok := node.Properties[objects.FieldKeyType].(string); ok {
		info.Type = typ
	}
	if opType, ok := node.Properties["operation_type"].(string); ok {
		info.OperationType = opType
	}
	if targetKind, ok := node.Properties[objects.FieldKeyTargetKind].(string); ok {
		info.TargetKind = targetKind
	}
	if filePath, ok := node.Properties[objects.FieldKeyFilePath].(string); ok {
		info.FilePath = filePath
	}

	return info
}

// TraversalFilter represents a filter in a traversal
type TraversalFilter struct {
	Type          FilterType
	TargetKinds   []string
	OperationType string
	NamePattern   string
}

// FilterType represents the type of filter
type FilterType string

const (
	FilterTypeTargetKind FilterType = "target_kind"
	FilterTypeOperation  FilterType = "operation"
	FilterTypeName       FilterType = "name"
)
