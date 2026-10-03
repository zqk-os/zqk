package memgraph

import (
	"context"

	"github.com/zqk-os/zqk/pkg/graph/provider"
)

const (
	defaultMemGraphPoolSize = 10
	providerNameMemGraph    = "MemGraph"
	providerVersion         = "1.0.0"
	providerDescription     = "MemGraph in-memory graph database provider with Cypher query support"
)

// MemGraphProvider implements the GraphProvider interface for MemGraph
type MemGraphProvider struct {
	config MemGraphConfig
}

// MemGraphConfig contains MemGraph-specific configuration
type MemGraphConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	Database string
	PoolSize int
}

// NewMemGraphProvider creates a new MemGraph provider
func NewMemGraphProvider(config *MemGraphConfig) *MemGraphProvider {
	return &MemGraphProvider{
		config: *config,
	}
}

// CreatePool creates a new connection pool to MemGraph
//
//nolint:gocritic // ConnectionConfig is shared interface type; keep value semantics
func (p *MemGraphProvider) CreatePool(ctx context.Context, config provider.ConnectionConfig) (provider.ConnectionPool, error) {
	mgConfig := MemGraphConfig{
		Host:     config.Host,
		Port:     config.Port,
		Username: config.Username,
		Password: config.Password,
		Database: config.Database,
		PoolSize: config.MaxConns,
	}
	if mgConfig.PoolSize <= 0 {
		mgConfig.PoolSize = defaultMemGraphPoolSize // Default
	}

	return NewMemGraphConnectionPool(&mgConfig)
}

// Connect is a convenience method that creates a pool with MaxConns=1
//
// Deprecated: Use CreatePool() instead for better resource management
//
//nolint:gocritic // ConnectionConfig matches provider interface; keep value semantics
func (p *MemGraphProvider) Connect(ctx context.Context, config provider.ConnectionConfig) (provider.GraphConnection, error) {
	conn, pool, err := provider.ConnectViaPool(ctx, config, p.CreatePool)
	if err != nil {
		return nil, err
	}

	// Return a wrapper that closes the pool when the connection is closed
	return &singleConnectionWrapper{
		conn: conn,
		pool: pool,
	}, nil
}

// singleConnectionWrapper wraps a connection from a single-connection pool
// When Close() is called, it returns the connection to the pool and closes the pool
type singleConnectionWrapper struct {
	conn provider.GraphConnection
	pool provider.ConnectionPool
}

func (w *singleConnectionWrapper) CreateNode(ctx context.Context, node provider.Node) error {
	return w.conn.CreateNode(ctx, node)
}

func (w *singleConnectionWrapper) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	return w.conn.GetNode(ctx, id, labels)
}

func (w *singleConnectionWrapper) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	return w.conn.UpdateNode(ctx, id, updates)
}

func (w *singleConnectionWrapper) DeleteNode(ctx context.Context, id string, labels []string) error {
	return w.conn.DeleteNode(ctx, id, labels)
}

func (w *singleConnectionWrapper) ListNodes(ctx context.Context, filter provider.NodeFilter) ([]*provider.Node, error) {
	return w.conn.ListNodes(ctx, filter)
}

func (w *singleConnectionWrapper) CreateEdge(ctx context.Context, edge provider.Edge) error {
	return w.conn.CreateEdge(ctx, edge)
}

func (w *singleConnectionWrapper) GetEdge(ctx context.Context, fromID, toID, edgeType string) (*provider.Edge, error) {
	return w.conn.GetEdge(ctx, fromID, toID, edgeType)
}

func (w *singleConnectionWrapper) UpdateEdge(ctx context.Context, fromID, toID, edgeType string, updates provider.EdgeUpdates) error {
	return w.conn.UpdateEdge(ctx, fromID, toID, edgeType, updates)
}

func (w *singleConnectionWrapper) DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error {
	return w.conn.DeleteEdge(ctx, fromID, toID, edgeType)
}

func (w *singleConnectionWrapper) ListEdges(ctx context.Context, filter provider.EdgeFilter) ([]*provider.Edge, error) {
	return w.conn.ListEdges(ctx, filter)
}

func (w *singleConnectionWrapper) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	return w.conn.ExecuteQuery(ctx, query)
}

//nolint:gocritic // provider interface uses value VectorQuery; keep signature
func (w *singleConnectionWrapper) ExecuteVectorQuery(ctx context.Context, query provider.VectorQuery) (*provider.QueryResult, error) {
	return w.conn.ExecuteVectorQuery(ctx, query)
}

//nolint:gocritic // provider interface uses value TraversalQuery; keep signature
func (w *singleConnectionWrapper) ExecuteTraversal(ctx context.Context, traversal provider.TraversalQuery) (*provider.QueryResult, error) {
	return w.conn.ExecuteTraversal(ctx, traversal)
}

func (w *singleConnectionWrapper) BeginTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	return w.conn.BeginTransaction(ctx)
}

func (w *singleConnectionWrapper) BeginNestedTransaction(ctx context.Context, parent provider.GraphTransaction) (provider.GraphTransaction, error) {
	return w.conn.BeginNestedTransaction(ctx, parent)
}

func (w *singleConnectionWrapper) HasOpenTransaction() bool {
	return w.conn.HasOpenTransaction()
}

func (w *singleConnectionWrapper) GetOpenTransaction() provider.GraphTransaction {
	return w.conn.GetOpenTransaction()
}

func (w *singleConnectionWrapper) HealthCheck(ctx context.Context) error {
	return w.conn.HealthCheck(ctx)
}

func (w *singleConnectionWrapper) Close() error {
	// Return connection to pool, then close pool
	//nolint:errcheck // Connection cleanup - error acceptable
	_ = w.pool.ReturnConnection(w.conn)
	return w.pool.Close() //nolint:gosec
}

func (w *singleConnectionWrapper) ExecuteBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	return w.conn.ExecuteBatch(ctx, operations)
}

// SupportsFeature checks if MemGraph supports a specific feature
func (p *MemGraphProvider) SupportsFeature(feature provider.Feature) bool {
	switch feature {
	case provider.FeatureCypherQuery,
		provider.FeatureVectorSearch,
		provider.FeatureMultiHopTraversal,
		provider.FeatureTransactions,
		provider.FeatureBatchOperations:
		return true
	default:
		return false
	}
}

// GetCapabilities returns MemGraph capabilities
func (p *MemGraphProvider) GetCapabilities() provider.Capabilities {
	return provider.Capabilities{
		Features: []provider.Feature{
			provider.FeatureCypherQuery,
			provider.FeatureVectorSearch,
			provider.FeatureMultiHopTraversal,
			provider.FeatureTransactions,
			provider.FeatureBatchOperations,
		},
		QueryLanguages: []provider.QueryLanguage{
			provider.QueryLanguageCypher,
		},
		TransactionSupport: true,
		VectorSearch:       true,
		MultiHopTraversal:  true,
		BatchOperations:    true,
		StreamingQueries:   false,
	}
}

// GetProviderInfo returns MemGraph provider information
func (p *MemGraphProvider) GetProviderInfo() provider.ProviderInfo {
	return provider.ProviderInfo{
		Name:        providerNameMemGraph,
		Version:     providerVersion,
		Description: providerDescription,
	}
}
