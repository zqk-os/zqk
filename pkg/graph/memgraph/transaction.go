package memgraph

import (
	"context"
	"fmt"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// memgraphTransaction implements the GraphTransaction interface for MemGraph
// Uses Bolt protocol explicit transactions for true ACID support
type memgraphTransaction struct {
	conn       *memgraphConnection
	parent     *memgraphTransaction // For nested transactions
	ctx        context.Context
	explicitTx neo4j.ExplicitTransaction // Bolt explicit transaction
	mu         sync.Mutex
	committed  bool
	rolledBack bool
}

// BeginTransaction starts a new transaction using Bolt managed transactions
func (c *memgraphConnection) BeginTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	if c.boltClient == nil {
		return nil, errfmt.Errorf("bolt client not initialized")
	}

	// Check if there's already an open transaction
	if c.HasOpenTransaction() {
		return nil, errfmt.Errorf("connection already has an open transaction")
	}

	// For Bolt protocol, we use explicit transactions via session.BeginTransaction
	// This gives us full control over commit/rollback
	session, err := c.getOrCreateSession(ctx)
	if err != nil {
		return nil, errfmt.Newf("failed to get session").Wrap(err)
	}

	if session == nil {
		return nil, errfmt.Errorf("session is nil - connection not properly initialized")
	}

	// Begin explicit transaction
	// Neo4j driver v5 uses a config function (can be nil for defaults)
	// Use recover to catch panics from nil driver/session
	var boltTx neo4j.ExplicitTransaction
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = errfmt.Errorf("panic during BeginTransaction: %v", r)
			}
		}()
		boltTx, err = session.BeginTransaction(ctx)
	}()

	if err != nil {
		return nil, errfmt.Newf("failed to begin transaction").Wrap(err)
	}

	// Create transaction object
	tx := &memgraphTransaction{
		conn:       c,
		ctx:        ctx,
		explicitTx: boltTx,
	}

	// Store transaction in connection
	c.SetOpenTransaction(tx)

	return tx, nil
}

// BeginNestedTransaction starts a nested transaction (savepoint)
func (c *memgraphConnection) BeginNestedTransaction(ctx context.Context, parent provider.GraphTransaction) (provider.GraphTransaction, error) {
	if c.boltClient == nil {
		return nil, errfmt.Errorf("bolt client not initialized")
	}

	// Type assert parent
	var parentTx *memgraphTransaction
	if parent != nil {
		var ok bool
		parentTx, ok = parent.(*memgraphTransaction)
		if !ok {
			return nil, errfmt.Errorf("invalid parent transaction type")
		}
	}

	// MemGraph doesn't support nested transactions via REST API
	// We'll simulate it by tracking the parent relationship
	tx := &memgraphTransaction{
		conn:   c,
		parent: parentTx,
		ctx:    ctx,
	}

	// Note: We don't replace the parent transaction in the connection
	// The parent remains the active transaction
	return tx, nil
}

// Commit commits the transaction
func (tx *memgraphTransaction) Commit(ctx context.Context) error {
	if tx.conn == nil {
		return errfmt.Errorf("transaction connection is nil")
	}

	var alreadyCommitted, alreadyRolledBack bool
	var explicitTx neo4j.ExplicitTransaction
	err := concurrency.RunInLockWithLogger(
		&tx.mu, LockNameMemgraphTxCommitCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			alreadyCommitted = tx.committed
			alreadyRolledBack = tx.rolledBack
			explicitTx = tx.explicitTx
			return nil
		},
	)
	if err != nil {
		return err
	}

	if alreadyCommitted {
		return errfmt.Errorf("transaction already committed")
	}
	if alreadyRolledBack {
		return errfmt.Errorf("transaction already rolled back")
	}

	if explicitTx == nil {
		return errfmt.Errorf("transaction not initialized")
	}

	// Commit the Bolt transaction (I/O outside lock)
	commitErr := explicitTx.Commit(ctx)
	if commitErr != nil {
		// Update state with lock
		_ = concurrency.RunInLockWithLogger(
			&tx.mu, LockNameMemgraphTxCommitFailed, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				tx.rolledBack = true
				return nil
			},
		)
		tx.conn.ClearTransaction()
		return errfmt.Newf("failed to commit transaction").Wrap(commitErr)
	}

	// Update state with lock
	err = concurrency.RunInLockWithLogger(
		&tx.mu, LockNameMemgraphTxCommitSuccess, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			tx.committed = true
			return nil
		},
	)
	if err != nil {
		return err
	}
	tx.conn.ClearTransaction()
	return nil
}

// Rollback rolls back the transaction
func (tx *memgraphTransaction) Rollback(ctx context.Context) error {
	if tx.conn == nil {
		return errfmt.Errorf("transaction connection is nil")
	}

	var alreadyCommitted, alreadyRolledBack bool
	var explicitTx neo4j.ExplicitTransaction
	err := concurrency.RunInLockWithLogger(
		&tx.mu, LockNameMemgraphTxRollbackCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			alreadyCommitted = tx.committed
			alreadyRolledBack = tx.rolledBack
			explicitTx = tx.explicitTx
			return nil
		},
	)
	if err != nil {
		return err
	}

	if alreadyCommitted {
		return errfmt.Errorf("transaction already committed")
	}
	if alreadyRolledBack {
		return nil // Already rolled back
	}

	if explicitTx == nil {
		_ = concurrency.RunInLockWithLogger(
			&tx.mu, LockNameMemgraphTxRollbackNoTx, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				tx.rolledBack = true
				return nil
			},
		)
		tx.conn.ClearTransaction()
		return nil
	}

	// Rollback the Bolt transaction (I/O outside lock)
	rollbackErr := explicitTx.Rollback(ctx)
	if rollbackErr != nil {
		return errfmt.Newf("failed to rollback transaction").Wrap(rollbackErr)
	}

	// Update state with lock
	err = concurrency.RunInLockWithLogger(
		&tx.mu, LockNameMemgraphTxRollbackSuccess, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			tx.rolledBack = true
			return nil
		},
	)
	if err != nil {
		return err
	}
	tx.conn.ClearTransaction()
	return nil
}

// SupportsNestedTransactions returns whether nested transactions are supported
func (tx *memgraphTransaction) SupportsNestedTransactions() bool {
	// MemGraph/Neo4j Bolt protocol doesn't support nested transactions (savepoints)
	return false
}

// BeginNestedTransaction starts a nested transaction
func (tx *memgraphTransaction) BeginNestedTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	return nil, errfmt.Errorf("nested transactions not supported by MemGraph Bolt protocol")
}

// GetParent returns the parent transaction if this is a nested transaction
func (tx *memgraphTransaction) GetParent() provider.GraphTransaction {
	return tx.parent
}

// IsCommitted returns whether the transaction has been committed
func (tx *memgraphTransaction) IsCommitted() bool {
	var committed bool
	_ = concurrency.RunInLockWithLogger(
		&tx.mu, LockNameMemgraphTxIsCommitted, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			committed = tx.committed
			return nil
		},
	)
	return committed
}

// IsRolledBack returns whether the transaction has been rolled back
func (tx *memgraphTransaction) IsRolledBack() bool {
	var rolledBack bool
	_ = concurrency.RunInLockWithLogger(
		&tx.mu, LockNameMemgraphTxIsRolledBack, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			rolledBack = tx.rolledBack
			return nil
		},
	)
	return rolledBack
}

// Transaction operations - execute within the managed transaction
// These operations are part of the transaction and will be committed/rolled back together

func (tx *memgraphTransaction) CreateNode(ctx context.Context, node provider.Node) error {
	if tx.explicitTx == nil {
		return errfmt.Errorf("transaction not initialized")
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
	for key, value := range node.Properties {
		props[key] = value
	}

	query := fmt.Sprintf("CREATE (n:%s $props) RETURN n", labels)
	params := map[string]any{
		"props": props,
	}

	// Execute within transaction
	_, err := tx.explicitTx.Run(ctx, query, params)
	return err
}

func (tx *memgraphTransaction) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	if tx.explicitTx == nil {
		return nil, errfmt.Errorf("transaction not initialized")
	}

	// Build label filter
	labelFilter := ""
	if len(labels) > 0 {
		labelFilter = ":" + labels[0]
		for i := 1; i < len(labels); i++ {
			labelFilter += ":" + labels[i]
		}
	}

	query := fmt.Sprintf("MATCH (n%s {id: $id}) RETURN n", labelFilter)
	params := map[string]any{
		objects.FieldKeyID: id,
	}

	result, err := tx.explicitTx.Run(ctx, query, params)
	if err != nil {
		return nil, err
	}

	records, err := result.Collect(ctx)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil // Node not found
	}

	nodeValue, ok := records[0].Values[0].(neo4j.Node)
	if !ok {
		return nil, errfmt.Errorf("unexpected node format in response")
	}

	return convertNeo4jNode(nodeValue), nil
}

func (tx *memgraphTransaction) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	if tx.explicitTx == nil {
		return errfmt.Errorf("transaction not initialized")
	}

	query := "MATCH (n {id: $id})"
	params := map[string]any{
		objects.FieldKeyID: id,
	}

	// Update properties
	if len(updates.Properties) > 0 {
		setClauses := []string{}
		for key, value := range updates.Properties {
			paramKey := fmt.Sprintf("prop_%s", key)
			setClauses = append(setClauses, fmt.Sprintf("n.%s = $%s", key, paramKey))
			params[paramKey] = value
		}
		if len(setClauses) > 0 {
			query += " SET " + setClauses[0]
			for i := 1; i < len(setClauses); i++ {
				query += ", " + setClauses[i]
			}
		}
	}

	// Add labels
	for _, label := range updates.AddLabels {
		query += fmt.Sprintf(" SET n:%s", label)
	}

	query += " RETURN n"

	_, err := tx.explicitTx.Run(ctx, query, params)
	return err
}

func (tx *memgraphTransaction) DeleteNode(ctx context.Context, id string, labels []string) error {
	if tx.explicitTx == nil {
		return errfmt.Errorf("transaction not initialized")
	}

	// Build label filter
	labelFilter := ""
	if len(labels) > 0 {
		labelFilter = ":" + labels[0]
		for i := 1; i < len(labels); i++ {
			labelFilter += ":" + labels[i]
		}
	}

	query := fmt.Sprintf("MATCH (n%s {id: $id}) DETACH DELETE n", labelFilter)
	params := map[string]any{
		objects.FieldKeyID: id,
	}

	_, err := tx.explicitTx.Run(ctx, query, params)
	return err
}

func (tx *memgraphTransaction) CreateEdge(ctx context.Context, edge provider.Edge) error {
	if tx.explicitTx == nil {
		return errfmt.Errorf("transaction not initialized")
	}

	query := "MATCH (a {id: $fromID}), (b {id: $toID}) CREATE (a)-[r:" + edge.Type + " $props]->(b) RETURN r"
	params := map[string]any{
		"fromID": edge.FromID,
		"toID":   edge.ToID,
		"props":  edge.Properties,
	}

	_, err := tx.explicitTx.Run(ctx, query, params)
	return err
}

func (tx *memgraphTransaction) GetEdge(ctx context.Context, fromID, toID, edgeType string) (*provider.Edge, error) {
	if tx.explicitTx == nil {
		return nil, errfmt.Errorf("transaction not initialized")
	}

	query := fmt.Sprintf("MATCH (a {id: $fromID})-[r:%s]->(b {id: $toID}) RETURN r, a.id AS fromID, b.id AS toID", edgeType)
	params := map[string]any{
		"fromID": fromID,
		"toID":   toID,
	}

	result, err := tx.explicitTx.Run(ctx, query, params)
	if err != nil {
		return nil, err
	}

	records, err := result.Collect(ctx)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil // Edge not found
	}

	record := records[0]
	relValue, ok := record.Values[0].(neo4j.Relationship)
	if !ok {
		return nil, errfmt.Errorf("unexpected edge format in response")
	}

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

func (tx *memgraphTransaction) UpdateEdge(ctx context.Context, fromID, toID, edgeType string, updates provider.EdgeUpdates) error {
	if tx.explicitTx == nil {
		return errfmt.Errorf("transaction not initialized")
	}

	query := fmt.Sprintf("MATCH (a {id: $fromID})-[r:%s]->(b {id: $toID})", edgeType)
	params := map[string]any{
		"fromID": fromID,
		"toID":   toID,
	}

	// Update properties
	if len(updates.Properties) > 0 {
		setClauses := []string{}
		for key, value := range updates.Properties {
			paramKey := fmt.Sprintf("prop_%s", key)
			setClauses = append(setClauses, fmt.Sprintf("r.%s = $%s", key, paramKey))
			params[paramKey] = value
		}
		if len(setClauses) > 0 {
			query += " SET " + setClauses[0]
			for i := 1; i < len(setClauses); i++ {
				query += ", " + setClauses[i]
			}
		}
	}

	query += " RETURN r"

	_, err := tx.explicitTx.Run(ctx, query, params)
	return err
}

func (tx *memgraphTransaction) DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error {
	if tx.explicitTx == nil {
		return errfmt.Errorf("transaction not initialized")
	}

	query := fmt.Sprintf("MATCH (a {id: $fromID})-[r:%s]->(b {id: $toID}) DELETE r", edgeType)
	params := map[string]any{
		"fromID": fromID,
		"toID":   toID,
	}

	_, err := tx.explicitTx.Run(ctx, query, params)
	return err
}

func (tx *memgraphTransaction) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	if tx.explicitTx == nil {
		return nil, errfmt.Errorf("transaction not initialized")
	}

	if query.Language != provider.QueryLanguageCypher {
		return nil, errfmt.Errorf("unsupported query language: %s", query.Language)
	}

	result, err := tx.explicitTx.Run(ctx, query.Query, query.Params)
	if err != nil {
		return nil, err
	}

	records, err := result.Collect(ctx)
	if err != nil {
		return nil, err
	}

	// Convert records to rows
	rows := make([]map[string]any, 0, len(records))
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
		Meta: make(map[string]any),
	}, nil
}
