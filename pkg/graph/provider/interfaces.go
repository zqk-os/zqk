package provider

import (
	"context"
	"fmt"
	"time"
)

// GraphProvider is the main interface for graph backend implementations.
// It provides factory methods for creating connection pools and querying backend capabilities.
type GraphProvider interface {
	// CreatePool creates a new connection pool to the graph backend
	// The pool manages connections internally and should be closed when done
	CreatePool(ctx context.Context, config ConnectionConfig) (ConnectionPool, error)

	// Connect is a convenience method that creates a pool with MaxConns=1 and returns a single connection
	// For production use, prefer CreatePool() for better resource management
	//
	// Deprecated: Use CreatePool() instead
	Connect(ctx context.Context, config ConnectionConfig) (GraphConnection, error)

	// SupportsFeature checks if the provider supports a specific feature
	SupportsFeature(feature Feature) bool

	// GetCapabilities returns provider capabilities and metadata
	GetCapabilities() Capabilities

	// GetProviderInfo returns provider name and version
	GetProviderInfo() ProviderInfo
}

// ProviderInfo contains provider metadata
type ProviderInfo struct {
	Name        string
	Version     string
	Description string
}

// Capabilities describes what features a provider supports
type Capabilities struct {
	Features           []Feature
	QueryLanguages     []QueryLanguage
	TransactionSupport bool
	VectorSearch       bool
	MultiHopTraversal  bool
	BatchOperations    bool
	StreamingQueries   bool
}

// Feature represents a capability that a provider may support
type Feature string

const (
	FeatureCypherQuery       Feature = "cypher_query"
	FeatureSPARQLQuery       Feature = "sparql_query"
	FeatureVectorSearch      Feature = "vector_search"
	FeatureMultiHopTraversal Feature = "multi_hop_traversal"
	FeatureTransactions      Feature = "transactions"
	FeatureBatchOperations   Feature = "batch_operations"
	FeatureStreamingQueries  Feature = "streaming_queries"
	FeatureGraphAlgorithms   Feature = "graph_algorithms"
)

// QueryLanguage represents a query language supported by a provider
type QueryLanguage string

const (
	QueryLanguageCypher  QueryLanguage = "cypher"
	QueryLanguageSPARQL  QueryLanguage = "sparql"
	QueryLanguageGremlin QueryLanguage = "gremlin"
	QueryLanguageNative  QueryLanguage = "native"
)

// ConnectionConfig contains connection parameters
type ConnectionConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	Database string
	MaxConns int

	// Timeout settings
	ConnectTimeout time.Duration // Timeout for establishing connections
	QueryTimeout   time.Duration // Default timeout for queries (can be overridden by context)
	PoolTimeout    time.Duration // Timeout for acquiring connection from pool

	// Retry configuration
	RetryConfig *RetryConfig // Default retry config for operations
}

// ConnectionPool manages a pool of graph connections.
// This is the primary interface for interacting with the graph backend.
// Connections are acquired from the pool, used, and returned when done.
type ConnectionPool interface {
	// GetConnection acquires a connection from the pool
	// The connection must be returned via ReturnConnection when done
	// If all connections are in use, this will block until one becomes available
	// The context timeout (from ConnectionConfig.PoolTimeout) is respected
	GetConnection(ctx context.Context) (GraphConnection, error)

	// ReturnConnection returns a connection to the pool
	// If the connection has an open transaction, it will be automatically rolled back
	// to prevent data inconsistency. This is a safety mechanism - transactions should
	// be explicitly committed or rolled back before returning the connection.
	ReturnConnection(conn GraphConnection) error

	// Execute executes an operation using a connection from the pool
	// This is a convenience method that handles connection acquisition/release automatically
	// Recommended for most use cases. Operations are auto-committed unless within a transaction.
	Execute(ctx context.Context, fn func(conn GraphConnection) error) error

	// Stats returns pool statistics for monitoring
	Stats() PoolStats

	// Close closes the pool and all connections
	// Any open transactions will be rolled back before closing
	Close() error
}

// PoolStats contains connection pool statistics
type PoolStats struct {
	Active    int   // Number of connections currently in use
	Idle      int   // Number of idle connections available
	MaxSize   int   // Maximum pool size
	WaitCount int64 // Number of times GetConnection had to wait
}

// GraphConnection represents a single connection from the pool.
// Connections are acquired from the pool and must be returned when done.
// Do not call Close() directly - use ConnectionPool.ReturnConnection() instead.
//
// **Auto-Commit Behavior**:
// - Operations outside of explicit transactions are automatically committed
// - Each operation (CreateNode, UpdateNode, etc.) is immediately committed
// - Transactions must be explicitly committed or rolled back
// - If a connection is returned to the pool with an open transaction, it is rolled back (safety mechanism)
type GraphConnection interface {
	// Node operations
	// These operations are auto-committed unless within an explicit transaction
	CreateNode(ctx context.Context, node Node) error
	GetNode(ctx context.Context, id string, labels []string) (*Node, error)
	UpdateNode(ctx context.Context, id string, updates NodeUpdates) error
	DeleteNode(ctx context.Context, id string, labels []string) error
	ListNodes(ctx context.Context, filter NodeFilter) ([]*Node, error)

	// Edge operations
	// These operations are auto-committed unless within an explicit transaction
	CreateEdge(ctx context.Context, edge Edge) error
	GetEdge(ctx context.Context, fromID, toID string, edgeType string) (*Edge, error)
	UpdateEdge(ctx context.Context, fromID, toID string, edgeType string, updates EdgeUpdates) error
	DeleteEdge(ctx context.Context, fromID, toID string, edgeType string) error
	ListEdges(ctx context.Context, filter EdgeFilter) ([]*Edge, error)

	// Query operations
	// Read operations (queries) are always auto-committed
	ExecuteQuery(ctx context.Context, query Query) (*QueryResult, error)
	ExecuteVectorQuery(ctx context.Context, query VectorQuery) (*QueryResult, error)
	ExecuteTraversal(ctx context.Context, traversal TraversalQuery) (*QueryResult, error)

	// Transaction management
	// BeginTransaction starts an explicit transaction. Operations within a transaction
	// are NOT auto-committed - you must call Commit() or Rollback() explicitly.
	BeginTransaction(ctx context.Context) (GraphTransaction, error)

	// BeginNestedTransaction starts a nested transaction (savepoint) if supported
	// parent is the parent transaction that this will be nested within
	// Returns error if nested transactions are not supported by the backend
	BeginNestedTransaction(ctx context.Context, parent GraphTransaction) (GraphTransaction, error)

	// Connection state queries
	HasOpenTransaction() bool
	GetOpenTransaction() GraphTransaction

	// HealthCheck checks if the connection is healthy
	HealthCheck(ctx context.Context) error

	// Close should not be called directly - use ConnectionPool.ReturnConnection() instead
	// This method exists for backward compatibility and will be removed in a future version
	//
	// Deprecated: Use ConnectionPool.ReturnConnection() instead
	Close() error

	// Batch operations
	// Batch operations are auto-committed unless within an explicit transaction
	ExecuteBatch(ctx context.Context, operations []Operation) (*BatchResult, error)
}

// Node represents a graph node
type Node struct {
	ID         string
	Labels     []string
	Properties map[string]any
}

// Edge represents a graph edge/relationship
type Edge struct {
	FromID     string
	ToID       string
	Type       string
	Properties map[string]any
}

// NodeUpdates contains fields to update on a node
type NodeUpdates struct {
	Properties   map[string]any
	AddLabels    []string
	RemoveLabels []string
}

// EdgeUpdates contains fields to update on an edge
type EdgeUpdates struct {
	Properties map[string]any
}

// NodeFilter filters nodes for queries
type NodeFilter struct {
	Labels     []string
	Properties map[string]any
	Limit      int
	Offset     int
}

// EdgeFilter filters edges for queries
type EdgeFilter struct {
	FromID     string
	ToID       string
	Type       string
	Properties map[string]any
	Limit      int
	Offset     int
}

// Query represents a graph query
type Query struct {
	Language QueryLanguage
	Query    string
	Params   map[string]any
}

// VectorQuery represents a vector similarity search query
type VectorQuery struct {
	Query     string
	Vector    []float32
	Limit     int
	Threshold float32
	Filter    NodeFilter
}

// TraversalQuery represents a graph traversal query
type TraversalQuery struct {
	StartNodeID  string
	Relationship string
	Direction    Direction
	MaxDepth     int
	Filter       NodeFilter
}

// Direction represents traversal direction
type Direction string

const (
	DirectionOutgoing Direction = "outgoing"
	DirectionIncoming Direction = "incoming"
	DirectionBoth     Direction = "both"
)

// QueryResult contains query execution results
type QueryResult struct {
	Nodes []*Node
	Edges []*Edge
	Rows  []map[string]any
	Meta  map[string]any
}

// GraphTransaction provides transaction support.
// Transactions must be explicitly committed or rolled back.
// If a connection is returned to the pool with an open transaction, it will be automatically rolled back.
//
// **Important**: Operations within a transaction are NOT auto-committed.
// You MUST call Commit() to persist changes or Rollback() to discard them.
type GraphTransaction interface {
	// Transaction operations (same as GraphConnection but within transaction)
	// These operations are NOT auto-committed - they are part of the transaction
	CreateNode(ctx context.Context, node Node) error
	GetNode(ctx context.Context, id string, labels []string) (*Node, error)
	UpdateNode(ctx context.Context, id string, updates NodeUpdates) error
	DeleteNode(ctx context.Context, id string, labels []string) error

	CreateEdge(ctx context.Context, edge Edge) error
	GetEdge(ctx context.Context, fromID, toID string, edgeType string) (*Edge, error)
	UpdateEdge(ctx context.Context, fromID, toID string, edgeType string, updates EdgeUpdates) error
	DeleteEdge(ctx context.Context, fromID, toID string, edgeType string) error

	ExecuteQuery(ctx context.Context, query Query) (*QueryResult, error)

	// Nested transaction support (savepoints)
	// BeginNestedTransaction creates a savepoint within this transaction
	// Returns error if nested transactions are not supported
	BeginNestedTransaction(ctx context.Context) (GraphTransaction, error)

	// SupportsNestedTransactions returns true if this transaction supports nested transactions
	SupportsNestedTransactions() bool

	// Transaction control
	// Commit persists all operations in the transaction
	// Rollback discards all operations in the transaction
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error

	// Transaction state queries
	IsCommitted() bool
	IsRolledBack() bool
	GetParent() GraphTransaction // Returns parent transaction if this is nested, nil otherwise
}

// Operation represents a single operation in a batch
type Operation struct {
	Type string // "create_node", "create_edge", "update_node", etc.
	Data any
}

// BatchResult contains batch operation results
type BatchResult struct {
	SuccessCount int
	FailureCount int
	Errors       []error
	Results      []any
}

// RetryConfig configures retry behavior for operations
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts (including initial attempt)
	// Default: 3
	MaxAttempts int

	// InitialDelay is the delay before the first retry
	// Default: 100ms
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries
	// Default: 5s
	MaxDelay time.Duration

	// BackoffFactor is the multiplier for exponential backoff
	// Default: 2.0
	BackoffFactor float64

	// RetryableErrors is a list of error codes that should trigger a retry
	// If empty, only transient errors (timeouts, connection failures) are retried
	RetryableErrors []ErrorCode
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
		RetryableErrors: []ErrorCode{
			ErrorCodeConnectionFailed,
			ErrorCodeTimeout,
			ErrorCodeQueryFailed, // Only if transient
		},
	}
}

// ErrorCode represents a type of error that can occur
type ErrorCode string

const (
	ErrorCodeConnectionFailed  ErrorCode = "connection_failed"
	ErrorCodeQueryFailed       ErrorCode = "query_failed"
	ErrorCodeNodeNotFound      ErrorCode = "node_not_found"
	ErrorCodeEdgeNotFound      ErrorCode = "edge_not_found"
	ErrorCodeTransactionFailed ErrorCode = "transaction_failed"
	ErrorCodeValidationFailed  ErrorCode = "validation_failed"
	ErrorCodeTimeout           ErrorCode = "timeout"
	ErrorCodeRetryExhausted    ErrorCode = "retry_exhausted"
	ErrorCodeDeadlineExceeded  ErrorCode = "deadline_exceeded"
)

// GraphError represents an error from the graph backend
type GraphError struct {
	Code    ErrorCode
	Message string
	Cause   error
	Backend string
}

func (e *GraphError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *GraphError) Unwrap() error {
	return e.Cause
}

// IsRetryable returns true if the error is transient and can be retried.
func (e *GraphError) IsRetryable() bool {
	retryableCodes := []ErrorCode{
		ErrorCodeConnectionFailed,
		ErrorCodeTimeout,
		ErrorCodeDeadlineExceeded,
		ErrorCodeRetryExhausted,
	}
	for _, code := range retryableCodes {
		if e.Code == code {
			return true
		}
	}
	return false
}

// IsNodeNotFound returns true if the error indicates a node was not found.
func (e *GraphError) IsNodeNotFound() bool {
	return e.Code == ErrorCodeNodeNotFound
}
