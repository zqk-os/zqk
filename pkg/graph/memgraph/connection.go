package memgraph

import (
	"context"
	"errors"
	"maps"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const (
	memgraphErrBoltClientNotInitialized = "bolt client not initialized"
	memgraphParamProps                  = "props"
	memgraphParamFromID                 = "fromID"
	memgraphParamToID                   = "toID"
	memgraphParamPropFmt                = "prop_%s"
	memgraphQueryWhere                  = " WHERE "
	memgraphQueryAnd                    = " AND "
	memgraphQuerySet                    = " SET "
	memgraphQueryReturnN                = " RETURN n"
	memgraphQueryLimitFmt               = " LIMIT %d"
	memgraphQuerySkipFmt                = " SKIP %d"
	memgraphSetNodePropFmt              = "n.`%s` = $%s"
	memgraphSetEdgePropFmt              = "r.`%s` = $%s"
	memgraphRemoveNodePropFmt           = " REMOVE n.`%s`"
)

// memgraphConnection implements the GraphConnection interface for MemGraph
// This is an internal type - users interact with the provider.GraphConnection interface
// It embeds BaseConnection to reuse shared transaction state management
type memgraphConnection struct {
	*provider.BaseConnection // Embedded for transaction state management
	boltClient               *boltClient
	session                  neo4j.SessionWithContext // Current session (if any)
	config                   MemGraphConfig
}

// buildLabelFilter returns a Cypher label filter like ":LabelA:LabelB" safely.
func buildLabelFilter(labels []string) string {
	if len(labels) == 0 {
		return ""
	}

	var b strings.Builder
	for _, label := range labels {
		if label == emptyValue {
			continue
		}
		b.WriteString(":")
		b.WriteString(label)
	}

	return b.String()
}

// Reset implements ConnectionWrapper.Reset
// BaseConnection.Reset() is called automatically, but we can add provider-specific reset logic here
func (c *memgraphConnection) Reset() {
	c.BaseConnection.Reset()
	if c.session != nil {
		_ = c.session.Close(context.Background())
		c.session = nil
	}
}

// CloseInternal implements ConnectionWrapper.CloseInternal
func (c *memgraphConnection) CloseInternal() error {
	// Rollback any open transaction (safety)
	if c.HasOpenTransaction() {
		if tx := c.GetOpenTransaction(); tx != nil {
			//nolint:errcheck // Intentional error ignored
			// Use system context for transaction rollback during connection cleanup
			_ = tx.Rollback(pkgctx.NewSystemContext())
		}
		c.ClearTransaction()
	}

	// Close session if open
	if c.session != nil {
		// Use system context for session close during connection cleanup
		c.session.Close(pkgctx.NewSystemContext()) //nolint:gosec
		c.session = nil
	}

	// Note: We don't close the driver here - it's managed by the pool
	return nil
}

// getOrCreateSession gets the current session or creates a new one
func (c *memgraphConnection) getOrCreateSession(ctx context.Context) (neo4j.SessionWithContext, error) {
	if c.session != nil {
		return c.session, nil
	}

	if c.boltClient == nil {
		return nil, errors.New(memgraphErrBoltClientNotInitialized)
	}

	if c.boltClient.driver == nil {
		return nil, errfmt.Errorf("bolt driver not initialized")
	}

	// Create a new session
	session := c.boltClient.newSession(ctx, neo4j.SessionConfig{})
	if session == nil {
		return nil, errfmt.Errorf("failed to create session - driver returned nil")
	}
	c.session = session
	return session, nil
}

// CreateNode creates a new node in the graph

func getNamespaceFromContext(ctx context.Context) string {
	if secCtx := pkgctx.GetSecurityContext(ctx); secCtx != nil {
		if secCtx.NamespaceID != "" && secCtx.NamespaceID != "*" {
			return secCtx.NamespaceID
		}
	}
	return ""
}

func (c *memgraphConnection) CreateNode(ctx context.Context, node provider.Node) error {
	if c.boltClient == nil {
		return errors.New(memgraphErrBoltClientNotInitialized)
	}

	// Build Cypher query
	labels := ""
	for i, label := range node.Labels {
		if i > 0 {
			labels += ":"
		}
		labels += label
	}

	// Build properties map
	props := map[string]any{
		objects.FieldKeyID: node.ID,
	}
	if ns := getNamespaceFromContext(ctx); ns != "" {
		props[objects.FieldKeyNamespaceID] = ns
	}
	for key, value := range node.Properties {
		props[key] = value
	}

	query := safeCypher("CREATE (n:%s $props) RETURN n", labels)
	params := map[string]any{
		memgraphParamProps: props,
	}

	// Check if we're in a transaction
	if c.HasOpenTransaction() {
		// If in transaction, delegate to transaction
		tx := c.GetOpenTransaction()
		if memTx, ok := tx.(*memgraphTransaction); ok {
			return memTx.CreateNode(ctx, node)
		}
	}

	// Auto-commit mode: use ExecuteWrite for write operations
	session, err := c.getOrCreateSession(ctx)
	if err != nil {
		return err
	}

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		_, err = result.Collect(ctx)
		return nil, err
	})

	return err
}

// GetNode retrieves a node by ID and labels
func (c *memgraphConnection) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	if c.boltClient == nil {
		return nil, errors.New(memgraphErrBoltClientNotInitialized)
	}

	// Build label filter
	labelFilter := buildLabelFilter(labels)

	query := safeCypher("MATCH (n%s {id: $id})", labelFilter)
	params := map[string]any{
		objects.FieldKeyID: id,
	}
	if ns := getNamespaceFromContext(ctx); ns != "" {
		query = safeCypher("MATCH (n%s {id: $id, namespace_id: $namespace_id})", labelFilter)
		params[objects.FieldKeyNamespaceID] = ns
	}
	query += " RETURN n"

	records, err := c.executeBoltQuery(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Parse response
	if len(records) == 0 {
		return nil, nil // Node not found
	}

	// Extract node from first record
	record := records[0]
	nodeValue, ok := record.Values[0].(neo4j.Node)
	if !ok {
		return nil, errfmt.Errorf("unexpected node format in response")
	}

	return convertNeo4jNode(nodeValue), nil
}

// UpdateNode updates a node's properties and labels
func (c *memgraphConnection) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	if c.boltClient == nil {
		return errors.New(memgraphErrBoltClientNotInitialized)
	}

	label := nodeLabelFromID(id)
	query := safeCypher("MATCH (n%s {id: $id})", label)
	params := map[string]any{
		objects.FieldKeyID: id,
	}

	// Update properties
	if len(updates.Properties) > 0 {
		setClauses := []string{}
		for key, value := range updates.Properties {
			paramKey := makeParamKey(key)
			setClauses = append(setClauses, safeCypher(memgraphSetNodePropFmt, key, paramKey))
			params[paramKey] = value
		}
		if len(setClauses) > 0 {
			query += memgraphQuerySet + setClauses[0]
			for i := 1; i < len(setClauses); i++ {
				query += ", " + setClauses[i]
			}
		}
	}

	// Add labels
	for _, label := range updates.AddLabels {
		query += safeCypher(" SET n:%s", label)
	}

	// Remove labels (Cypher doesn't have direct REMOVE LABEL, so we use a workaround)
	// For now, we'll skip label removal as it requires more complex Cypher

	// FieldUnset: SET-merge cannot drop keys; REMOVE the property on the node.
	// TRACK: BLI-1785439369431933000-f0cccd6c
	for _, key := range updates.RemoveProperties {
		query += safeCypher(memgraphRemoveNodePropFmt, key)
	}

	query += memgraphQueryReturnN

	_, err := c.executeBoltQuery(ctx, query, params)
	return err
}

// DeleteNode deletes a node from the graph
func (c *memgraphConnection) DeleteNode(ctx context.Context, id string, labels []string) error {
	if c.boltClient == nil {
		return errors.New(memgraphErrBoltClientNotInitialized)
	}

	// Build label filter
	labelFilter := buildLabelFilter(labels)

	query := safeCypher("MATCH (n%s {id: $id})", labelFilter)
	params := map[string]any{
		objects.FieldKeyID: id,
	}
	if ns := getNamespaceFromContext(ctx); ns != "" {
		query = safeCypher("MATCH (n%s {id: $id, namespace_id: $namespace_id})", labelFilter)
		params[objects.FieldKeyNamespaceID] = ns
	}
	query += " DETACH DELETE n"

	_, err := c.executeBoltQuery(ctx, query, params)
	return err
}

// ListNodes lists nodes matching the filter
func (c *memgraphConnection) ListNodes(ctx context.Context, filter provider.NodeFilter) ([]*provider.Node, error) {
	if c.boltClient == nil {
		return nil, errors.New(memgraphErrBoltClientNotInitialized)
	}

	// Build label filter
	labelFilter := buildLabelFilter(filter.Labels)

	query := safeCypher("MATCH (n%s)", labelFilter)
	params := map[string]any{}

	// Add property filters
	whereClauses := []string{}
	for key, value := range filter.Properties {
		paramKey := makeParamKey(key)
		whereClauses = append(whereClauses, safeCypher(memgraphSetNodePropFmt, key, paramKey))
		params[paramKey] = value
	}

	if len(whereClauses) > 0 {
		query += memgraphQueryWhere + whereClauses[0]
		for i := 1; i < len(whereClauses); i++ {
			query += memgraphQueryAnd + whereClauses[i]
		}
	}

	query += memgraphQueryReturnN

	// Add limit and offset
	if filter.Limit > 0 {
		query += safeCypher(memgraphQueryLimitFmt, filter.Limit)
	}
	if filter.Offset > 0 {
		query += safeCypher(memgraphQuerySkipFmt, filter.Offset)
	}

	records, err := c.executeBoltQuery(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Convert results to nodes
	nodes := []*provider.Node{}
	for _, record := range records {
		nodeValue, ok := record.Values[0].(neo4j.Node)
		if !ok {
			continue
		}

		node := convertNeo4jNode(nodeValue)
		if node.ID != emptyValue {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// CreateEdge creates a new edge/relationship in the graph
func (c *memgraphConnection) CreateEdge(ctx context.Context, edge provider.Edge) error {
	if c.boltClient == nil {
		return errors.New(memgraphErrBoltClientNotInitialized)
	}

	fromLabel := nodeLabelFromID(edge.FromID)
	toLabel := nodeLabelFromID(edge.ToID)
	query := safeCypher("MATCH (a%s {id: $fromID}), (b%s {id: $toID}) CREATE (a)-[r:%s $props]->(b) RETURN r", fromLabel, toLabel, edge.Type)
	params := map[string]any{
		memgraphParamFromID: edge.FromID,
		memgraphParamToID:   edge.ToID,
		memgraphParamProps:  edge.Properties,
	}

	_, err := c.executeBoltQuery(ctx, query, params)
	return err
}

// GetEdge retrieves an edge by from/to IDs and type
func (c *memgraphConnection) GetEdge(ctx context.Context, fromID, toID, edgeType string) (*provider.Edge, error) {
	if c.boltClient == nil {
		return nil, errors.New(memgraphErrBoltClientNotInitialized)
	}

	fromLabel := nodeLabelFromID(fromID)
	toLabel := nodeLabelFromID(toID)
	query := safeCypher("MATCH (a%s {id: $fromID})-[r:%s]->(b%s {id: $toID}) RETURN r, a.id AS fromID, b.id AS toID", fromLabel, edgeType, toLabel)
	params := map[string]any{
		memgraphParamFromID: fromID,
		memgraphParamToID:   toID,
	}

	records, err := c.executeBoltQuery(ctx, query, params)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil // Edge not found
	}

	// Extract edge from first record
	record := records[0]
	relValue, ok := record.Values[0].(neo4j.Relationship)
	if !ok {
		return nil, errfmt.Errorf("unexpected edge format in response")
	}

	// Get from/to IDs from record
	recordFromID := fromID
	recordToID := toID
	if len(record.Values) > 1 {
		if f, ok := record.Values[1].(string); ok {
			recordFromID = f
		}
	}
	if len(record.Values) > 2 {
		if t, ok := record.Values[2].(string); ok {
			recordToID = t
		}
	}

	return convertNeo4jRelationship(relValue, recordFromID, recordToID), nil
}

// UpdateEdge updates an edge's properties
func (c *memgraphConnection) UpdateEdge(ctx context.Context, fromID, toID, edgeType string, updates provider.EdgeUpdates) error {
	if c.boltClient == nil {
		return errors.New(memgraphErrBoltClientNotInitialized)
	}

	fromLabel := nodeLabelFromID(fromID)
	toLabel := nodeLabelFromID(toID)
	query := safeCypher("MATCH (a%s {id: $fromID})-[r:%s]->(b%s {id: $toID})", fromLabel, edgeType, toLabel)
	params := map[string]any{
		memgraphParamFromID: fromID,
		memgraphParamToID:   toID,
	}

	// Update properties
	if len(updates.Properties) > 0 {
		setClauses := []string{}
		for key, value := range updates.Properties {
			paramKey := makeParamKey(key)
			setClauses = append(setClauses, safeCypher(memgraphSetEdgePropFmt, key, paramKey))
			params[paramKey] = value
		}
		if len(setClauses) > 0 {
			query += memgraphQuerySet + setClauses[0]
			for i := 1; i < len(setClauses); i++ {
				query += ", " + setClauses[i]
			}
		}
	}

	query += " RETURN r"

	_, err := c.executeBoltQuery(ctx, query, params)
	return err
}

// DeleteEdge deletes an edge from the graph
func (c *memgraphConnection) DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error {
	if c.boltClient == nil {
		return errors.New(memgraphErrBoltClientNotInitialized)
	}

	fromLabel := nodeLabelFromID(fromID)
	toLabel := nodeLabelFromID(toID)
	query := safeCypher("MATCH (a%s {id: $fromID})-[r:%s]->(b%s {id: $toID}) DELETE r", fromLabel, edgeType, toLabel)
	params := map[string]any{
		memgraphParamFromID: fromID,
		memgraphParamToID:   toID,
	}

	_, err := c.executeBoltQuery(ctx, query, params)
	return err
}

// ListEdges lists edges matching the filter
//
//nolint:gocyclo // Edge listing supports many filter combinations; refactor later
func (c *memgraphConnection) ListEdges(ctx context.Context, filter provider.EdgeFilter) ([]*provider.Edge, error) {
	if c.boltClient == nil {
		return nil, errors.New(memgraphErrBoltClientNotInitialized)
	}

	query := "MATCH (a)-[r"
	if filter.Type != emptyValue {
		query += ":" + filter.Type
	}
	query += "]->(b)"

	params := map[string]any{}
	whereClauses := []string{}

	// Add from/to filters
	if filter.FromID != emptyValue {
		whereClauses = append(whereClauses, "a.id = $fromID")
		params[memgraphParamFromID] = filter.FromID
	}
	if filter.ToID != emptyValue {
		whereClauses = append(whereClauses, "b.id = $toID")
		params[memgraphParamToID] = filter.ToID
	}

	// Add property filters
	for key, value := range filter.Properties {
		paramKey := makeParamKey(key)
		whereClauses = append(whereClauses, safeCypher(memgraphSetEdgePropFmt, key, paramKey))
		params[paramKey] = value
	}

	if len(whereClauses) > 0 {
		query += memgraphQueryWhere + whereClauses[0]
		for i := 1; i < len(whereClauses); i++ {
			query += memgraphQueryAnd + whereClauses[i]
		}
	}

	query += " RETURN a.id AS fromID, b.id AS toID, type(r) AS type, r AS properties"

	// Add limit and offset
	if filter.Limit > 0 {
		query += safeCypher(memgraphQueryLimitFmt, filter.Limit)
	}
	if filter.Offset > 0 {
		query += safeCypher(memgraphQuerySkipFmt, filter.Offset)
	}

	records, err := c.executeBoltQuery(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Convert results to edges
	edges := []*provider.Edge{}
	for _, record := range records {
		if len(record.Values) < 4 {
			continue
		}

		fromID, _ := record.Values[0].(string)
		toID, _ := record.Values[1].(string)
		edgeType, _ := record.Values[2].(string)
		relValue, ok := record.Values[3].(neo4j.Relationship)

		if fromID != emptyValue && toID != emptyValue && edgeType != emptyValue && ok {
			edge := convertNeo4jRelationship(relValue, fromID, toID)
			edges = append(edges, edge)
		}
	}

	return edges, nil
}

// ExecuteQuery executes a Cypher query
func (c *memgraphConnection) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	if c.boltClient == nil {
		return nil, errors.New(memgraphErrBoltClientNotInitialized)
	}

	if query.Language != provider.QueryLanguageCypher {
		return nil, errfmt.Errorf("unsupported query language: %s", query.Language)
	}

	records, err := c.executeBoltQuery(ctx, query.Query, query.Params)
	if err != nil {
		return nil, err
	}

	// Convert records to rows (map[string]any)
	// Also extract nodes and convert them to provider.Node format
	rows := make([]map[string]any, 0, len(records))
	nodes := make([]*provider.Node, 0)

	for _, record := range records {
		row := make(map[string]any)
		for i, key := range record.Keys {
			if i < len(record.Values) {
				value := record.Values[i]
				// Convert neo4j.Node to provider.Node if found
				if neo4jNode, ok := value.(neo4j.Node); ok {
					convertedNode := convertNeo4jNode(neo4jNode)
					row[key] = convertedNode
					// Also add to nodes list if it's the main result (typically "n")
					if key == "n" || len(nodes) == 0 {
						nodes = append(nodes, convertedNode)
					}
				} else {
					row[key] = value
				}
			}
		}
		rows = append(rows, row)
	}

	// Convert response to QueryResult
	result := &provider.QueryResult{
		Nodes: nodes,
		Rows:  rows,
		Meta:  make(map[string]any),
	}

	return result, nil
}

// ExecuteVectorQuery executes a vector similarity search query
//
//nolint:gocyclo,gocritic // Complex query construction; provider interface uses value VectorQuery
func (c *memgraphConnection) ExecuteVectorQuery(ctx context.Context, query provider.VectorQuery) (*provider.QueryResult, error) {
	if c.boltClient == nil {
		return nil, errors.New(memgraphErrBoltClientNotInitialized)
	}

	// MemGraph supports vector similarity search via Cypher extensions
	// The query should use MemGraph's vector similarity functions
	// If a custom query is provided, use it; otherwise build a standard one

	cypherQuery := query.Query
	if cypherQuery == emptyValue {
		// Build standard vector similarity query
		// MemGraph uses vector.similarity() function
		labelFilter := ""
		if len(query.Filter.Labels) > 0 {
			labelFilter = ":" + query.Filter.Labels[0]
			for i := 1; i < len(query.Filter.Labels); i++ {
				labelFilter += ":" + query.Filter.Labels[i]
			}
		}

		cypherQuery = safeCypher("MATCH (n%s)", labelFilter)

		// Add property filters
		whereClauses := []string{}
		params := make(map[string]any)

		// Vector similarity condition
		whereClauses = append(whereClauses, "vector.similarity(n.embedding, $vector) > $threshold")
		params["vector"] = query.Vector
		params["threshold"] = query.Threshold

		// Add property filters
		for key, value := range query.Filter.Properties {
			paramKey := makeParamKey(key)
			whereClauses = append(whereClauses, safeCypher(memgraphSetNodePropFmt, key, paramKey))
			params[paramKey] = value
		}

		if len(whereClauses) > 0 {
			cypherQuery += memgraphQueryWhere + whereClauses[0]
			for i := 1; i < len(whereClauses); i++ {
				cypherQuery += memgraphQueryAnd + whereClauses[i]
			}
		}

		cypherQuery += " RETURN n, vector.similarity(n.embedding, $vector) AS similarity"

		if query.Limit > 0 {
			cypherQuery += safeCypher(" ORDER BY similarity DESC LIMIT %d", query.Limit)
		} else {
			cypherQuery += " ORDER BY similarity DESC"
		}

		records, err := c.executeBoltQuery(ctx, cypherQuery, params)
		if err != nil {
			return nil, errfmt.Newf("vector query execution failed").Wrap(err)
		}

		// Convert results
		nodes := []*provider.Node{}
		rows := []map[string]any{}

		for _, record := range records {
			row := make(map[string]any)
			for i, key := range record.Keys {
				if i < len(record.Values) {
					row[key] = record.Values[i]
					// Extract node if present
					if key == "n" {
						if nodeValue, ok := record.Values[i].(neo4j.Node); ok {
							nodes = append(nodes, convertNeo4jNode(nodeValue))
						}
					}
				}
			}
			rows = append(rows, row)
		}

		return &provider.QueryResult{
			Nodes: nodes,
			Rows:  rows,
			Meta: map[string]any{
				"query_type": "vector_similarity",
				"threshold":  query.Threshold,
			},
		}, nil
	}

	// Use custom query if provided
	params := map[string]any{
		"vector":    query.Vector,
		"threshold": query.Threshold,
	}

	// Merge filter properties into params
	for key, value := range query.Filter.Properties {
		params[key] = value
	}

	records, err := c.executeBoltQuery(ctx, cypherQuery, params)
	if err != nil {
		return nil, errfmt.Newf("vector query execution failed").Wrap(err)
	}

	// Convert results
	rows := []map[string]any{}
	for _, record := range records {
		row := make(map[string]any)
		for i, key := range record.Keys {
			if i < len(record.Values) {
				row[key] = record.Values[i]
			}
		}
		rows = append(rows, row)
	}

	return &provider.QueryResult{
		Rows: rows,
		Meta: map[string]any{
			"query_type": "vector_similarity",
			"threshold":  query.Threshold,
		},
	}, nil
}

// ExecuteTraversal executes a graph traversal query
//
//nolint:gocyclo,gocritic // Traversal query building is branching; interface uses value TraversalQuery
func (c *memgraphConnection) ExecuteTraversal(ctx context.Context, traversal provider.TraversalQuery) (*provider.QueryResult, error) {
	if c.boltClient == nil {
		return nil, errors.New(memgraphErrBoltClientNotInitialized)
	}

	// Build Cypher query for traversal
	// Direction mapping
	var relPattern string
	relType := ""
	if traversal.Relationship != "" {
		relType = ":" + traversal.Relationship
	}
	switch traversal.Direction {
	case provider.DirectionOutgoing:
		relPattern = safeCypher("-[r%s*1..%d]->", relType, traversal.MaxDepth)
	case provider.DirectionIncoming:
		relPattern = safeCypher("<-[r%s*1..%d]-", relType, traversal.MaxDepth)
	case provider.DirectionBoth:
		relPattern = safeCypher("-[r%s*1..%d]-", relType, traversal.MaxDepth)
	default:
		relPattern = safeCypher("-[*1..%d]-", traversal.MaxDepth)
	}

	// Build label filter for start node
	startLabels := ""
	if len(traversal.Filter.Labels) > 0 {
		startLabels = ":" + traversal.Filter.Labels[0]
		for i := 1; i < len(traversal.Filter.Labels); i++ {
			startLabels += ":" + traversal.Filter.Labels[i]
		}
	}

	// Build query
	query := safeCypher("MATCH path = (start%s {id: $startID})%s(end) RETURN path, start, end, relationships(path) AS rels", startLabels, relPattern)
	params := map[string]any{
		"startID": traversal.StartNodeID,
	}

	// Add property filters if specified
	if len(traversal.Filter.Properties) > 0 {
		whereClauses := []string{}
		for key, value := range traversal.Filter.Properties {
			paramKey := makeParamKey(key)
			whereClauses = append(whereClauses, safeCypher("end.`%s` = $%s", key, paramKey))
			params[paramKey] = value
		}
		if len(whereClauses) > 0 {
			query += " WHERE " + whereClauses[0]
			for i := 1; i < len(whereClauses); i++ {
				query += " AND " + whereClauses[i]
			}
		}
	}

	records, err := c.executeBoltQuery(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Extract nodes and edges from path results
	nodes := []*provider.Node{}
	edges := []*provider.Edge{}
	rows := []map[string]any{}

	for _, record := range records {
		row := make(map[string]any)

		// Extract path (if present)
		if len(record.Values) > 0 {
			if pathValue, ok := record.Values[0].(neo4j.Path); ok {
				// Extract nodes from path
				for _, node := range pathValue.Nodes {
					nodes = append(nodes, convertNeo4jNode(node))
				}
				// Extract relationships from path
				// Neo4j Path contains relationships with start/end node element IDs
				for i, rel := range pathValue.Relationships {
					var fromID, toID string

					// Get start and end node IDs from the relationship
					// Relationships in a path are ordered, so we can infer from/to from node positions
					if i < len(pathValue.Nodes)-1 {
						fromNode := convertNeo4jNode(pathValue.Nodes[i])
						toNode := convertNeo4jNode(pathValue.Nodes[i+1])
						fromID = fromNode.ID
						toID = toNode.ID
					} else if len(pathValue.Nodes) > 0 {
						// Fallback: use first and last nodes
						fromNode := convertNeo4jNode(pathValue.Nodes[0])
						toNode := convertNeo4jNode(pathValue.Nodes[len(pathValue.Nodes)-1])
						fromID = fromNode.ID
						toID = toNode.ID
					}

					edges = append(edges, convertNeo4jRelationship(rel, fromID, toID))
				}
			}
		}

		// Build row from all record values
		for i, key := range record.Keys {
			if i < len(record.Values) {
				row[key] = record.Values[i]
			}
		}
		rows = append(rows, row)
	}

	return &provider.QueryResult{
		Nodes: nodes,
		Edges: edges,
		Rows:  rows,
		Meta:  make(map[string]any),
	}, nil
}

// BeginTransaction starts a new transaction
// Implementation is in transaction.go
// BeginNestedTransaction starts a nested transaction (savepoint)
// Implementation is in transaction.go

// HasOpenTransaction and GetOpenTransaction are provided by BaseConnection
// No need to reimplement them

// HealthCheck checks if the connection is healthy
func (c *memgraphConnection) HealthCheck(ctx context.Context) error {
	if c.boltClient == nil {
		return errfmt.Errorf("bolt client not initialized")
	}
	return c.boltClient.verifyConnectivity(ctx)
}

// Close closes the connection
// This method exists for backward compatibility
//
// Deprecated: Use ConnectionPool.ReturnConnection() instead
func (c *memgraphConnection) Close() error {
	// Rollback any open transaction
	if c.HasOpenTransaction() {
		if tx := c.GetOpenTransaction(); tx != nil {
			//nolint:errcheck // Intentional error ignored
			// Use system context for transaction rollback during connection cleanup
			_ = tx.Rollback(pkgctx.NewSystemContext())
		}
		c.ClearTransaction()
	}
	return c.CloseInternal()
}

// ExecuteBatch executes a batch of operations
//
//nolint:gocyclo // Batch execution covers multiple operation types; acceptable complexity
func (c *memgraphConnection) ExecuteBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	if c.boltClient == nil {
		return nil, errfmt.Errorf("bolt client not initialized")
	}

	if len(operations) == 0 {
		return &provider.BatchResult{
			SuccessCount: 0,
			FailureCount: 0,
			Errors:       []error{},
			Results:      []any{},
		}, nil
	}

	// Check if we're in a transaction - if not, start one for batch operations
	var tx provider.GraphTransaction
	var startedTx bool
	var err error

	if !c.HasOpenTransaction() {
		tx, err = c.BeginTransaction(ctx)
		if err != nil {
			return nil, errfmt.Newf("failed to begin transaction for batch").Wrap(err)
		}
		startedTx = true
	} else {
		tx = c.GetOpenTransaction()
	}

	startTime := time.Now()
	chunkCount := 0
	fallbackSingleCount := 0

	result := &provider.BatchResult{
		SuccessCount: 0,
		FailureCount: 0,
		Errors:       []error{},
		Results:      []any{},
	}

	collector := provider.GetGlobalGraphProviderMetricsCollector()

	defer func() {
		var finalErr error
		if result != nil && len(result.Errors) > 0 {
			finalErr = result.Errors[0]
		}
		collector.RecordBatch(len(operations), time.Since(startTime), chunkCount, fallbackSingleCount > 0, finalErr)
	}()

	// Execute each operation in the batch
	for i := 0; i < len(operations); {
		op := operations[i]
		var opErr error
		var opResult any

		// Try to chunk create_node operations
		if op.Type == "create_node" {
			if _, ok := op.Data.(provider.Node); ok {
				chunkEnd := i + 1
				for chunkEnd < len(operations) && chunkEnd-i < 500 {
					if operations[chunkEnd].Type != "create_node" {
						break
					}
					if _, nextOk := operations[chunkEnd].Data.(provider.Node); !nextOk {
						break
					}
					chunkEnd++
				}

				if chunkEnd-i > 1 { // Batch them!
					chunkCount++

					// Group by labelFilter to execute one UNWIND per unique label combination
					labelGroups := make(map[string][]int) // labelFilter -> indices
					for j := i; j < chunkEnd; j++ {
						currNode := operations[j].Data.(provider.Node)
						lf := buildLabelFilter(currNode.Labels)
						labelGroups[lf] = append(labelGroups[lf], j)
					}

					batchSuccess := true
					for lf, indices := range labelGroups {
						batchData := make([]map[string]any, 0, len(indices))
						for _, j := range indices {
							currNode := operations[j].Data.(provider.Node)
							props := map[string]any{
								objects.FieldKeyID: currNode.ID,
							}
							for k, v := range currNode.Properties {
								props[k] = v
							}
							batchData = append(batchData, props)
						}

						query := safeCypher("UNWIND $batch AS row MERGE (n%s {id: row.id}) SET n = row RETURN n", lf)
						_, opErr = tx.ExecuteQuery(ctx, provider.Query{
							Language: provider.QueryLanguageCypher,
							Query:    query,
							Params: map[string]any{
								"batch": batchData,
							},
						})

						if opErr != nil {
							batchSuccess = false
							break
						}
					}

					if !batchSuccess {
						result.FailureCount++
						result.Errors = append(result.Errors, errfmt.Errorf("batch create_node operation failed at index %d: %w", i, opErr))
						if startedTx && tx != nil {
							//nolint:errcheck // Intentional error ignored
							_ = tx.Rollback(ctx)
							return result, errfmt.Errorf("batch operation failed at index %d: %w", i, opErr)
						}
						break
					} else {
						result.SuccessCount += (chunkEnd - i)
						for j := i; j < chunkEnd; j++ {
							result.Results = append(result.Results, operations[j].Data)
						}
					}
					i = chunkEnd
					continue
				}
			}
		}

		// Try to chunk create_edge operations
		if op.Type == "create_edge" {
			if edge, ok := op.Data.(provider.Edge); ok {
				edgeType := edge.Type
				fromLabel := nodeLabelFromID(edge.FromID)
				toLabel := nodeLabelFromID(edge.ToID)
				chunkEnd := i + 1
				for chunkEnd < len(operations) && chunkEnd-i < 500 {
					if operations[chunkEnd].Type != "create_edge" {
						break
					}
					nextEdge, nextOk := operations[chunkEnd].Data.(provider.Edge)
					if !nextOk || nextEdge.Type != edgeType || nodeLabelFromID(nextEdge.FromID) != fromLabel || nodeLabelFromID(nextEdge.ToID) != toLabel {
						break
					}
					chunkEnd++
				}

				if chunkEnd-i > 1 {
					chunkCount++
					batchData := make([]map[string]any, 0, chunkEnd-i)
					for j := i; j < chunkEnd; j++ {
						currEdge := operations[j].Data.(provider.Edge)
						row := map[string]any{
							memgraphParamFromID: currEdge.FromID,
							memgraphParamToID:   currEdge.ToID,
							memgraphParamProps:  currEdge.Properties,
						}
						batchData = append(batchData, row)
					}

					query := safeCypher("UNWIND $batch AS row MATCH (a%s {id: row.fromID}), (b%s {id: row.toID}) MERGE (a)-[r:%s]->(b) SET r = row.props RETURN r", fromLabel, toLabel, edgeType)
					_, opErr = tx.ExecuteQuery(ctx, provider.Query{
						Language: provider.QueryLanguageCypher,
						Query:    query,
						Params: map[string]any{
							"batch": batchData,
						},
					})

					if opErr != nil {
						result.FailureCount++
						result.Errors = append(result.Errors, errfmt.Errorf("batch create_edge operation failed at index %d: %w", i, opErr))
						if startedTx && tx != nil {
							//nolint:errcheck // Intentional error ignored
							_ = tx.Rollback(ctx)
							return result, errfmt.Errorf("batch operation failed at index %d: %w", i, opErr)
						}
						break
					} else {
						result.SuccessCount += (chunkEnd - i)
						for j := i; j < chunkEnd; j++ {
							result.Results = append(result.Results, operations[j].Data)
						}
					}
					i = chunkEnd
					continue
				}
			}
		}

		// Chunk update_node operations into UNWIND SET grouped by label filter.
		if op.Type == "update_node" {
			if _, ok := op.Data.(map[string]any); ok {
				chunkEnd := i + 1
				for chunkEnd < len(operations) && chunkEnd-i < 500 {
					if operations[chunkEnd].Type != "update_node" {
						break
					}
					if _, nextOk := operations[chunkEnd].Data.(map[string]any); !nextOk {
						break
					}
					chunkEnd++
				}
				if chunkEnd-i > 1 {
					chunkCount++

					labelGroups := make(map[string][]int) // labelFilter -> indices
					for j := i; j < chunkEnd; j++ {
						curr := operations[j].Data.(map[string]any)
						currID, _ := curr[objects.FieldKeyID].(string)
						lf := nodeLabelFromID(currID)
						labelGroups[lf] = append(labelGroups[lf], j)
					}

					batchSuccess := true
					for lf, indices := range labelGroups {
						batchData := make([]map[string]any, 0, len(indices))
						for _, j := range indices {
							curr := operations[j].Data.(map[string]any)
							currID, _ := curr[objects.FieldKeyID].(string)
							props, _ := curr["properties"].(map[string]any)
							if props == nil {
								props = map[string]any{}
							}
							rowProps := make(map[string]any, len(props)+1)
							maps.Copy(rowProps, props)
							rowProps[objects.FieldKeyID] = currID
							batchData = append(batchData, map[string]any{
								objects.FieldKeyID: currID,
								"props":            rowProps,
							})
						}

						query := safeCypher("UNWIND $batch AS row MATCH (n%s {id: row.id}) SET n = row.props RETURN n", lf)
						_, opErr = tx.ExecuteQuery(ctx, provider.Query{
							Language: provider.QueryLanguageCypher,
							Query:    query,
							Params:   map[string]any{"batch": batchData},
						})

						if opErr != nil {
							batchSuccess = false
							break
						}
					}

					if !batchSuccess {
						result.FailureCount++
						result.Errors = append(result.Errors, errfmt.Errorf("batch update_node operation failed at index %d: %w", i, opErr))
						if startedTx && tx != nil {
							//nolint:errcheck // Intentional error ignored
							_ = tx.Rollback(ctx)
							return result, errfmt.Errorf("batch operation failed at index %d: %w", i, opErr)
						}
						break
					}
					result.SuccessCount += chunkEnd - i
					for j := i; j < chunkEnd; j++ {
						result.Results = append(result.Results, operations[j].Data)
					}
					i = chunkEnd
					continue
				}
			}
		}

		fallbackSingleCount++
		switch op.Type {
		case "create_node":
			if node, ok := op.Data.(provider.Node); ok {
				opErr = tx.CreateNode(ctx, node)
				opResult = node
			} else {
				opErr = errfmt.Errorf("invalid node data for create_node operation")
			}

		case "update_node":
			if updateData, ok := op.Data.(map[string]any); ok {
				id, _ := updateData[objects.FieldKeyID].(string)
				updates := provider.NodeUpdates{}
				if props, ok := updateData["properties"].(map[string]any); ok {
					updates.Properties = props
				}
				if labels, ok := updateData["add_labels"].([]string); ok {
					updates.AddLabels = labels
				}
				opErr = tx.UpdateNode(ctx, id, updates)
				opResult = updateData
			} else {
				opErr = errfmt.Errorf("invalid update data for update_node operation")
			}

		case "delete_node":
			if deleteData, ok := op.Data.(map[string]any); ok {
				id, _ := deleteData[objects.FieldKeyID].(string)
				labels := []string{}
				if labelsData, ok := deleteData["labels"].([]string); ok {
					labels = labelsData
				}
				opErr = tx.DeleteNode(ctx, id, labels)
				opResult = deleteData
			} else {
				opErr = errfmt.Errorf("invalid delete data for delete_node operation")
			}

		case "create_edge":
			if edge, ok := op.Data.(provider.Edge); ok {
				opErr = tx.CreateEdge(ctx, edge)
				opResult = edge
			} else {
				opErr = errfmt.Errorf("invalid edge data for create_edge operation")
			}

		case "update_edge":
			if updateData, ok := op.Data.(map[string]any); ok {
				fromID, _ := updateData["from_id"].(string)
				toID, _ := updateData["to_id"].(string)
				edgeType, _ := updateData[objects.FieldKeyType].(string)
				updates := provider.EdgeUpdates{}
				if props, ok := updateData["properties"].(map[string]any); ok {
					updates.Properties = props
				}
				opErr = tx.UpdateEdge(ctx, fromID, toID, edgeType, updates)
				opResult = updateData
			} else {
				opErr = errfmt.Errorf("invalid update data for update_edge operation")
			}

		case "delete_edge":
			if deleteData, ok := op.Data.(map[string]any); ok {
				fromID, _ := deleteData["from_id"].(string)
				toID, _ := deleteData["to_id"].(string)
				edgeType, _ := deleteData[objects.FieldKeyType].(string)
				opErr = tx.DeleteEdge(ctx, fromID, toID, edgeType)
				opResult = deleteData
			} else {
				opErr = errfmt.Errorf("invalid delete data for delete_edge operation")
			}

		case "execute_query":
			if queryData, ok := op.Data.(provider.Query); ok {
				var queryResult *provider.QueryResult
				queryResult, opErr = tx.ExecuteQuery(ctx, queryData)
				opResult = queryResult
			} else {
				opErr = errfmt.Errorf("invalid query data for execute_query operation")
			}

		default:
			opErr = errfmt.Errorf("unknown operation type: %s", op.Type)
		}

		if opErr != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, errfmt.Errorf("operation %d (%s) failed: %w", i, op.Type, opErr))
			// If we started the transaction, rollback on first error
			if startedTx && tx != nil {
				//nolint:errcheck // Intentional error ignored
				_ = tx.Rollback(ctx)
				return result, errfmt.Errorf("batch operation failed at index %d: %w", i, opErr)
			}
		} else {
			result.SuccessCount++
			result.Results = append(result.Results, opResult)
		}
		i++
	}

	// Commit transaction if we started it
	if startedTx && tx != nil {
		if err = tx.Commit(ctx); err != nil {
			//nolint:errcheck // Intentional error ignored
			_ = tx.Rollback(ctx)
			return result, errfmt.Newf("failed to commit batch transaction").Wrap(err)
		}
	}

	return result, nil
}

// nodeLabelFromID determines the node label from its ID namespace prefix
func nodeLabelFromID(id string) string {
	if strings.HasPrefix(id, "code_entity:") {
		return ":CodeEntity"
	}
	if strings.HasPrefix(id, "source_file:") {
		return ":SourceFile"
	}
	if strings.HasPrefix(id, "package:") {
		return ":Package"
	}
	return ""
}

func sanitizeParamKey(key string) string {
	var sb strings.Builder
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

func makeParamKey(key string) string {
	return "prop_" + sanitizeParamKey(key)
}
