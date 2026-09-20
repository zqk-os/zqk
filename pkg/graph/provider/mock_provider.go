package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// MockGraphProvider implements GraphProvider for testing purposes.
type MockGraphProvider struct {
	store *MockStore
}

// NewMockGraphProvider creates a new MockGraphProvider.
func NewMockGraphProvider() *MockGraphProvider {
	return &MockGraphProvider{
		store: NewMockStore(),
	}
}

// CreatePool creates a new MockConnectionPool.
func (p *MockGraphProvider) CreatePool(ctx context.Context, config ConnectionConfig) (ConnectionPool, error) {
	pool := &MockConnectionPool{
		BasePool: NewBasePool(&config, config.MaxConns),
		provider: p,
	}
	return pool, nil
}

// Connect creates a single MockConnection.
func (p *MockGraphProvider) Connect(ctx context.Context, config ConnectionConfig) (GraphConnection, error) {
	config.MaxConns = 1
	pool, err := p.CreatePool(ctx, config)
	if err != nil {
		return nil, err
	}
	return pool.GetConnection(ctx)
}

// SupportsFeature checks if the mock provider supports a feature.
func (p *MockGraphProvider) SupportsFeature(feature Feature) bool {
	switch feature {
	case FeatureCypherQuery, FeatureTransactions:
		return true
	default:
		return false
	}
}

// GetCapabilities returns the capabilities of the mock provider.
func (p *MockGraphProvider) GetCapabilities() Capabilities {
	return Capabilities{
		Features:           []Feature{FeatureCypherQuery, FeatureTransactions},
		QueryLanguages:     []QueryLanguage{QueryLanguageCypher},
		TransactionSupport: true,
	}
}

// GetProviderInfo returns provider metadata.
func (p *MockGraphProvider) GetProviderInfo() ProviderInfo {
	return ProviderInfo{
		Name:        "MockProvider",
		Version:     "1.0.0",
		Description: "Persistent mock graph provider for testing",
	}
}

// MockConnectionPool implements ConnectionPool using BasePool.
type MockConnectionPool struct {
	*BasePool
	provider *MockGraphProvider
}

// GetConnection acquires a connection from the pool.
func (p *MockConnectionPool) GetConnection(ctx context.Context) (GraphConnection, error) {
	return p.BasePool.GetConnection(ctx, func() (ConnectionWrapper, error) {
		return &MockConnection{
			store: p.provider.store,
		}, nil
	})
}

// ReturnConnection returns a connection to the pool.
func (p *MockConnectionPool) ReturnConnection(conn GraphConnection) error {
	if cw, ok := conn.(ConnectionWrapper); ok {
		return p.BasePool.ReturnConnection(cw)
	}
	return nil
}

// Execute executes an operation using a connection from the pool.
func (p *MockConnectionPool) Execute(ctx context.Context, fn func(conn GraphConnection) error) error {
	conn := &MockConnection{
		store: p.provider.store,
	}
	return fn(conn)
}

// Stats returns pool statistics.
func (p *MockConnectionPool) Stats() PoolStats {
	return PoolStats{}
}

// Close closes the pool.
func (p *MockConnectionPool) Close() error {
	return nil
}

// MockStore maintains an in-memory representation of nodes and edges.
type MockStore struct {
	mu          sync.RWMutex
	nodes       map[string]*Node
	edges       map[string]*Edge
	accelerator *EmbeddedGraphAccelerator
}

// Accelerator returns the embedded graph accelerator.
func (s *MockStore) Accelerator() *EmbeddedGraphAccelerator {
	return s.accelerator
}

// NewMockStore creates a new MockStore and loads from disk if available.
func NewMockStore() *MockStore {
	store := &MockStore{
		nodes:       make(map[string]*Node),
		edges:       make(map[string]*Edge),
		accelerator: NewEmbeddedGraphAccelerator(DefaultMaxTraversalBound, DefaultTraversalTimeout),
	}
	store.loadFromDisk()
	return store
}

func (s *MockStore) loadFromDisk() {
	path := s.getPersistPath()
	if path == "" {
		return
	}
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return
	}
	var state struct {
		Nodes map[string]*Node `json:"nodes"`
		Edges map[string]*Edge `json:"edges"`
	}
	if err := json.Unmarshal(data, &state); err == nil {
		if state.Nodes != nil {
			s.nodes = state.Nodes
			for _, node := range s.nodes {
				s.accelerator.IndexNode(node)
			}
		}
		if state.Edges != nil {
			s.edges = state.Edges
			for _, edge := range s.edges {
				s.accelerator.IndexEdge(edge)
			}
		}
	}
}

func (s *MockStore) saveToDisk() {
	path := s.getPersistPath()
	if path == "" {
		return
	}
	state := struct {
		Nodes map[string]*Node `json:"nodes"`
		Edges map[string]*Edge `json:"edges"`
	}{
		Nodes: s.nodes,
		Edges: s.edges,
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	_ = fileutil.EnsureDir(filepath.Dir(path))
	_ = fileutil.WriteSecureFile(path, data)
}

func (s *MockStore) getPersistPath() string {
	// Use .zqk/cache/mock_graph.json
	projectRoot := zqkenv.ProjectRoot().Get()
	if projectRoot == "" {
		// Prevent tests from polluting package directories with .zqk folders
		if strings.HasSuffix(os.Args[0], ".test") {
			return ""
		}
		projectRoot = "."
	}
	// Verify directory exists
	cacheDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir)
	if _, err := fileutil.Stat(cacheDir); err != nil {
		// Try to create it if it doesn't exist
		_ = fileutil.EnsureDir(cacheDir)
	}
	return filepath.Join(cacheDir, "mock_graph.json")
}

func edgeKey(fromID, toID, edgeType string) string {
	return fmt.Sprintf("%s|%s|%s", fromID, toID, edgeType)
}

// MockConnection implements GraphConnection.
type MockConnection struct {
	BaseConnection
	store *MockStore
}

// CloseInternal is called by the pool when closing a connection.
func (c *MockConnection) CloseInternal() error {
	return nil
}

// Reset resets the connection state.
func (c *MockConnection) Reset() {
	c.BaseConnection.Reset()
}

// Close is deprecated but implemented for interface compatibility.
func (c *MockConnection) Close() error {
	return nil
}

// Node operations

// CreateNode adds a node to the store.
func (c *MockConnection) CreateNode(ctx context.Context, node Node) error {
	if tx := c.GetOpenTransaction(); tx != nil {
		return tx.CreateNode(ctx, node)
	}
	return concurrency.RunInLockWithLogger(
		&c.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.store.nodes[node.ID] = &node
			c.store.accelerator.IndexNode(&node)
			c.store.saveToDisk()
			return nil
		},
	)
}

// GetNode retrieves a node from the store.
func (c *MockConnection) GetNode(ctx context.Context, id string, labels []string) (*Node, error) {
	if tx := c.GetOpenTransaction(); tx != nil {
		return tx.GetNode(ctx, id, labels)
	}
	var node *Node
	err := concurrency.RunInRLockWithLogger(
		&c.store.mu, "mock_store_read", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			node, ok = c.store.nodes[id]
			if !ok {
				return &GraphError{Code: ErrorCodeNodeNotFound, Message: fmt.Sprintf("node %s not found", id)}
			}
			return nil
		},
	)
	return node, err
}

// UpdateNode updates a node in the store.
func (c *MockConnection) UpdateNode(ctx context.Context, id string, updates NodeUpdates) error {
	if tx := c.GetOpenTransaction(); tx != nil {
		return tx.UpdateNode(ctx, id, updates)
	}
	return concurrency.RunInLockWithLogger(
		&c.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			node, ok := c.store.nodes[id]
			if !ok {
				return &GraphError{Code: ErrorCodeNodeNotFound, Message: fmt.Sprintf("node %s not found", id)}
			}
			// Apply updates (simplified)
			if updates.Properties != nil {
				if node.Properties == nil {
					node.Properties = make(map[string]any)
				}
				for k, v := range updates.Properties {
					node.Properties[k] = v
				}
			}
			for _, k := range updates.RemoveProperties {
				delete(node.Properties, k)
			}
			if len(updates.AddLabels) > 0 {
				node.Labels = append(node.Labels, updates.AddLabels...)
			}

			c.store.accelerator.IndexNode(node)
			c.store.saveToDisk()
			return nil
		},
	)
}

// DeleteNode removes a node from the store.
func (c *MockConnection) DeleteNode(ctx context.Context, id string, labels []string) error {
	if tx := c.GetOpenTransaction(); tx != nil {
		return tx.DeleteNode(ctx, id, labels)
	}
	return concurrency.RunInLockWithLogger(
		&c.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(c.store.nodes, id)
			c.store.accelerator.RemoveNode(id)
			c.store.saveToDisk()
			return nil
		},
	)
}

// ListNodes lists nodes matching the filter using the vertex-label index when labels are specified.
func (c *MockConnection) ListNodes(ctx context.Context, filter NodeFilter) ([]*Node, error) {
	// Accelerated index path when labels are present: zero full-graph scan
	if len(filter.Labels) > 0 {
		candidates, err := c.store.accelerator.LookupByLabel(ctx, filter.Labels[0], filter.Limit)
		if err != nil {
			return nil, err
		}
		if len(filter.Labels) == 1 && len(filter.Properties) == 0 {
			return candidates, nil
		}
		var result []*Node
		for _, node := range candidates {
			match := true
			for _, fl := range filter.Labels[1:] {
				hasLabel := false
				for _, nl := range node.Labels {
					if nl == fl {
						hasLabel = true
						break
					}
				}
				if !hasLabel {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			for k, v := range filter.Properties {
				if pv, ok := node.Properties[k]; !ok || pv != v {
					match = false
					break
				}
			}
			if match {
				result = append(result, node)
			}
		}
		return result, nil
	}

	var result []*Node
	err := concurrency.RunInRLockWithLogger(
		&c.store.mu, "mock_store_read", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, node := range c.store.nodes {
				if len(filter.Properties) > 0 {
					match := true
					for k, v := range filter.Properties {
						if pv, ok := node.Properties[k]; !ok || pv != v {
							match = false
							break
						}
					}
					if !match {
						continue
					}
				}
				result = append(result, node)
			}
			return nil
		},
	)
	return result, err
}

// Edge operations

// CreateEdge adds an edge to the store.
func (c *MockConnection) CreateEdge(ctx context.Context, edge Edge) error {
	if tx := c.GetOpenTransaction(); tx != nil {
		return tx.CreateEdge(ctx, edge)
	}
	return concurrency.RunInLockWithLogger(
		&c.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.store.edges[edgeKey(edge.FromID, edge.ToID, edge.Type)] = &edge
			c.store.accelerator.IndexEdge(&edge)
			c.store.saveToDisk()
			return nil
		},
	)
}

// GetEdge retrieves an edge from the store.
func (c *MockConnection) GetEdge(ctx context.Context, fromID, toID string, edgeType string) (*Edge, error) {
	if tx := c.GetOpenTransaction(); tx != nil {
		return tx.GetEdge(ctx, fromID, toID, edgeType)
	}
	var edge *Edge
	err := concurrency.RunInRLockWithLogger(
		&c.store.mu, "mock_store_read", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			edge, ok = c.store.edges[edgeKey(fromID, toID, edgeType)]
			if !ok {
				return &GraphError{Code: ErrorCodeEdgeNotFound, Message: "edge not found"}
			}
			return nil
		},
	)
	return edge, err
}

// UpdateEdge updates an edge in the store.
func (c *MockConnection) UpdateEdge(ctx context.Context, fromID, toID string, edgeType string, updates EdgeUpdates) error {
	if tx := c.GetOpenTransaction(); tx != nil {
		return tx.UpdateEdge(ctx, fromID, toID, edgeType, updates)
	}
	return concurrency.RunInLockWithLogger(
		&c.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			edge, ok := c.store.edges[edgeKey(fromID, toID, edgeType)]
			if !ok {
				return &GraphError{Code: ErrorCodeEdgeNotFound, Message: "edge not found"}
			}
			if updates.Properties != nil {
				if edge.Properties == nil {
					edge.Properties = make(map[string]any)
				}
				for k, v := range updates.Properties {
					edge.Properties[k] = v
				}
			}
			c.store.saveToDisk()
			return nil
		},
	)
}

// DeleteEdge removes an edge from the store.
func (c *MockConnection) DeleteEdge(ctx context.Context, fromID, toID string, edgeType string) error {
	if tx := c.GetOpenTransaction(); tx != nil {
		return tx.DeleteEdge(ctx, fromID, toID, edgeType)
	}
	return concurrency.RunInLockWithLogger(
		&c.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(c.store.edges, edgeKey(fromID, toID, edgeType))
			c.store.accelerator.RemoveEdge(fromID, toID, edgeType)
			c.store.saveToDisk()
			return nil
		},
	)
}

// ListEdges lists edges matching the filter.
func (c *MockConnection) ListEdges(ctx context.Context, filter EdgeFilter) ([]*Edge, error) {
	var result []*Edge
	err := concurrency.RunInRLockWithLogger(
		&c.store.mu, "mock_store_read", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, edge := range c.store.edges {
				if filter.FromID != "" && edge.FromID != filter.FromID {
					continue
				}
				if filter.ToID != "" && edge.ToID != filter.ToID {
					continue
				}
				if filter.Type != "" && edge.Type != filter.Type {
					continue
				}
				// Simplified property matching
				if len(filter.Properties) > 0 {
					match := true
					for k, v := range filter.Properties {
						if ev, ok := edge.Properties[k]; !ok || ev != v {
							match = false
							break
						}
					}
					if !match {
						continue
					}
				}
				result = append(result, edge)
			}
			return nil
		},
	)
	return result, err
}

// ExecuteQuery executes a query (minimal support).
func (c *MockConnection) ExecuteQuery(ctx context.Context, query Query) (*QueryResult, error) {
	return c.executeQueryInternal(ctx, query, true)
}

// executeQueryInternal executes a query (minimal support).
func (c *MockConnection) executeQueryInternal(ctx context.Context, query Query, checkTx bool) (*QueryResult, error) {
	if checkTx {
		if tx := c.GetOpenTransaction(); tx != nil {
			return tx.ExecuteQuery(ctx, query)
		}
	}

	// Basic Cypher-like label matching support for testing via embedded accelerator
	// e.g., "MATCH (n:BacklogItem) RETURN n"
	if strings.Contains(query.Query, "MATCH (n:") {
		res, err := c.store.accelerator.ExecuteCypher(ctx, query.Query, query.Params)
		if err == nil && res != nil {
			return res, nil
		}
	}

	const (
		propOwner     = "owner"
		propExpiresAt = "expiresAt"
	)

	if strings.Contains(query.Query, "MERGE (l:Lock") {
		resourceID, _ := query.Params["resourceID"].(string)
		ownerID, _ := query.Params["ownerID"].(string)

		now := getInt64(query.Params["now"])
		expiresAt := getInt64(query.Params["expiresAt"])

		var acquired bool
		_ = concurrency.RunInLockWithLogger(
			&c.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				nodeID := "lock:" + resourceID
				node, exists := c.store.nodes[nodeID]
				if !exists {
					c.store.nodes[nodeID] = &Node{
						ID:         nodeID,
						Labels:     []string{"Lock"},
						Properties: map[string]any{objects.FieldKeyID: resourceID, propOwner: ownerID, propExpiresAt: expiresAt},
					}
					acquired = true
				} else {
					currentOwner, _ := node.Properties[propOwner].(string)
					currentExpires := getInt64(node.Properties[propExpiresAt])

					if currentExpires < now {
						node.Properties[propOwner] = ownerID
						node.Properties[propExpiresAt] = expiresAt
						acquired = true
					} else if currentOwner == ownerID {
						node.Properties[propExpiresAt] = expiresAt
						acquired = true
					} else {
						acquired = false
					}
				}
				return nil
			},
		)

		return &QueryResult{
			Rows: []map[string]any{
				{"acquired": acquired},
			},
		}, nil
	}

	if strings.Contains(query.Query, "DELETE l") && strings.Contains(query.Query, "MATCH (l:Lock") {
		resourceID, _ := query.Params["resourceID"].(string)
		ownerID, _ := query.Params["ownerID"].(string)

		_ = concurrency.RunInLockWithLogger(
			&c.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				nodeID := "lock:" + resourceID
				if node, exists := c.store.nodes[nodeID]; exists {
					if node.Properties[propOwner] == ownerID {
						delete(c.store.nodes, nodeID)
					}
				}
				return nil
			},
		)
		return &QueryResult{}, nil
	}

	// Return all nodes if query is very broad
	if strings.Contains(query.Query, "MATCH (n) RETURN n") || strings.Contains(query.Query, "MATCH (n:Entity) RETURN n") {
		var nodes []*Node
		_ = concurrency.RunInRLockWithLogger(
			&c.store.mu, "mock_store_read", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				for _, node := range c.store.nodes {
					nodes = append(nodes, node)
				}
				return nil
			},
		)
		return &QueryResult{Nodes: nodes}, nil
	}

	// Return empty result for unsupported queries
	return &QueryResult{}, nil
}

// getInt64 abstracts the common pattern of safely casting an 'any' value
// (from JSON unmarshaling or mock parameters) to an int64.
func getInt64(val any) int64 {
	switch v := val.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	default:
		return 0
	}
}

// ExecuteVectorQuery executes a vector query.
func (c *MockConnection) ExecuteVectorQuery(ctx context.Context, query VectorQuery) (*QueryResult, error) {
	return &QueryResult{}, nil
}

// ExecuteTraversal executes a traversal query using the bound 1-hop edge traverser.
func (c *MockConnection) ExecuteTraversal(ctx context.Context, traversal TraversalQuery) (*QueryResult, error) {
	return c.store.accelerator.Traverse1Hop(ctx, traversal)
}

// Accelerator returns the embedded graph accelerator.
func (c *MockConnection) Accelerator() *EmbeddedGraphAccelerator {
	return c.store.accelerator
}


// BeginTransaction starts an explicit transaction.
func (c *MockConnection) BeginTransaction(ctx context.Context) (GraphTransaction, error) {
	tx := &MockTransaction{
		conn: c,
	}
	c.SetOpenTransaction(tx)
	return tx, nil
}

// BeginNestedTransaction starts a nested transaction (not supported).
func (c *MockConnection) BeginNestedTransaction(ctx context.Context, parent GraphTransaction) (GraphTransaction, error) {
	return nil, fmt.Errorf("nested transactions not supported")
}

// HealthCheck checks the connection health.
func (c *MockConnection) HealthCheck(ctx context.Context) error {
	return nil
}

// ExecuteBatch executes multiple operations.
func (c *MockConnection) ExecuteBatch(ctx context.Context, operations []Operation) (*BatchResult, error) {
	result := &BatchResult{
		Results: make([]any, len(operations)),
	}
	for i, op := range operations {
		var err error
		switch op.Type {
		case "create_node":
			if node, ok := op.Data.(Node); ok {
				err = c.CreateNode(ctx, node)
			}
		case "create_edge":
			if edge, ok := op.Data.(Edge); ok {
				err = c.CreateEdge(ctx, edge)
			}
		// Add more cases as needed
		default:
			err = fmt.Errorf("unsupported batch operation: %s", op.Type)
		}

		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, err)
		} else {
			result.SuccessCount++
		}
		result.Results[i] = err
	}
	return result, nil
}

// MockTransaction implements GraphTransaction.
type MockTransaction struct {
	conn       *MockConnection
	ops        []func() error
	isFinished bool
}

func (t *MockTransaction) CreateNode(ctx context.Context, node Node) error {
	t.ops = append(t.ops, func() error {
		return concurrency.RunInLockWithLogger(
			&t.conn.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				t.conn.store.nodes[node.ID] = &node
				t.conn.store.saveToDisk()
				return nil
			},
		)
	})
	return nil
}

func (t *MockTransaction) GetNode(ctx context.Context, id string, labels []string) (*Node, error) {
	// For mock transactions, we can just read from the store
	// (Note: this doesn't see uncommitted changes in the same transaction)
	var node *Node
	err := concurrency.RunInRLockWithLogger(
		&t.conn.store.mu, "mock_store_read", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			node, ok = t.conn.store.nodes[id]
			if !ok {
				return &GraphError{Code: ErrorCodeNodeNotFound, Message: fmt.Sprintf("node %s not found", id)}
			}
			return nil
		},
	)
	return node, err
}

func (t *MockTransaction) UpdateNode(ctx context.Context, id string, updates NodeUpdates) error {
	t.ops = append(t.ops, func() error {
		return concurrency.RunInLockWithLogger(
			&t.conn.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				node, ok := t.conn.store.nodes[id]
				if !ok {
					return &GraphError{Code: ErrorCodeNodeNotFound, Message: fmt.Sprintf("node %s not found", id)}
				}
				// Apply updates (simplified)
				if updates.Properties != nil {
					if node.Properties == nil {
						node.Properties = make(map[string]any)
					}
					for k, v := range updates.Properties {
						node.Properties[k] = v
					}

				}
				for _, k := range updates.RemoveProperties {
					delete(node.Properties, k)
				}
				if len(updates.AddLabels) > 0 {
					node.Labels = append(node.Labels, updates.AddLabels...)
				}
				t.conn.store.saveToDisk()
				return nil
			},
		)
	})
	return nil
}

func (t *MockTransaction) DeleteNode(ctx context.Context, id string, labels []string) error {
	t.ops = append(t.ops, func() error {
		return concurrency.RunInLockWithLogger(
			&t.conn.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				delete(t.conn.store.nodes, id)
				t.conn.store.saveToDisk()
				return nil
			},
		)
	})
	return nil
}

func (t *MockTransaction) CreateEdge(ctx context.Context, edge Edge) error {
	t.ops = append(t.ops, func() error {
		return concurrency.RunInLockWithLogger(
			&t.conn.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				t.conn.store.edges[edgeKey(edge.FromID, edge.ToID, edge.Type)] = &edge
				t.conn.store.saveToDisk()
				return nil
			},
		)
	})
	return nil
}

func (t *MockTransaction) GetEdge(ctx context.Context, fromID, toID string, edgeType string) (*Edge, error) {
	var edge *Edge
	err := concurrency.RunInRLockWithLogger(
		&t.conn.store.mu, "mock_store_read", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			edge, ok = t.conn.store.edges[edgeKey(fromID, toID, edgeType)]
			if !ok {
				return &GraphError{Code: ErrorCodeEdgeNotFound, Message: "edge not found"}
			}
			return nil
		},
	)
	return edge, err
}

func (t *MockTransaction) UpdateEdge(ctx context.Context, fromID, toID string, edgeType string, updates EdgeUpdates) error {
	t.ops = append(t.ops, func() error {
		return concurrency.RunInLockWithLogger(
			&t.conn.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				edge, ok := t.conn.store.edges[edgeKey(fromID, toID, edgeType)]
				if !ok {
					return &GraphError{Code: ErrorCodeEdgeNotFound, Message: "edge not found"}
				}
				if updates.Properties != nil {
					if edge.Properties == nil {
						edge.Properties = make(map[string]any)
					}
					for k, v := range updates.Properties {
						edge.Properties[k] = v
					}
				}
				t.conn.store.saveToDisk()
				return nil
			},
		)
	})
	return nil
}

func (t *MockTransaction) DeleteEdge(ctx context.Context, fromID, toID string, edgeType string) error {
	t.ops = append(t.ops, func() error {
		return concurrency.RunInLockWithLogger(
			&t.conn.store.mu, "mock_store_write", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				delete(t.conn.store.edges, edgeKey(fromID, toID, edgeType))
				t.conn.store.saveToDisk()
				return nil
			},
		)
	})
	return nil
}

func (t *MockTransaction) ExecuteQuery(ctx context.Context, query Query) (*QueryResult, error) {
	// For mock, just delegate to the connection (ignore transactional isolation for queries in mock)
	return t.conn.executeQueryInternal(ctx, query, false)
}

func (t *MockTransaction) BeginNestedTransaction(ctx context.Context) (GraphTransaction, error) {
	return nil, fmt.Errorf("nested transactions not supported")
}

func (t *MockTransaction) SupportsNestedTransactions() bool {
	return false
}

func (t *MockTransaction) Commit(ctx context.Context) error {
	if t.isFinished {
		return fmt.Errorf("transaction already finished")
	}
	for _, op := range t.ops {
		if err := op(); err != nil {
			return err
		}
	}
	t.isFinished = true
	t.conn.ClearTransaction()
	return nil
}

func (t *MockTransaction) Rollback(ctx context.Context) error {
	if t.isFinished {
		return fmt.Errorf("transaction already finished")
	}
	t.isFinished = true
	t.conn.ClearTransaction()
	return nil
}

func (t *MockTransaction) IsCommitted() bool {
	return t.isFinished // Simplified
}

func (t *MockTransaction) IsRolledBack() bool {
	return t.isFinished // Simplified
}

func (t *MockTransaction) GetParent() GraphTransaction {
	return nil
}
