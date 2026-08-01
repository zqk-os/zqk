package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// Re-export context types for convenience
type SecurityContext = pkgctx.SecurityContext
type StorageContext = pkgctx.StorageContext

// ObjectStorageProvider defines the interface for object storage implementations
// This allows consistent behavior across file-based and graph-based backends
// CacheRefreshPolicy added for cache refresh policies
type CacheRefreshPolicy string

const DefaultCacheRefreshPolicy CacheRefreshPolicy = "default"

type ObjectStorageProvider interface {
	// Create creates a new object
	// - Validates object before persistence
	// - Enforces permissions (write permission for object kind)
	// - Generates ID if not provided
	// - Sets created_at, created_by, updated_at, updated_by
	// - Returns error if object already exists
	Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error

	// Read retrieves an object by ID
	// - Enforces permissions (read permission for object kind)
	// - Returns error if object not found
	Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error)

	// Update updates an existing object
	// - Validates object before persistence
	// - Enforces permissions (write permission for object kind)
	// - Uses optimistic locking (checks updated_at timestamp)
	// - Sets updated_at, updated_by
	// - Returns error if object not found or version conflict
	Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error

	// Delete deletes an object
	// - Enforces permissions (delete permission for object kind)
	// - Handles cascade deletes (deletes dependent objects)
	// - Returns error if object not found or has dependencies
	Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error

	// List lists objects with filtering, sorting, pagination, and grouping
	// - Enforces permissions (read permission for object kind)
	// - Supports filtering by any field
	// - Supports sorting by any field
	// - Supports pagination (offset, limit) - respects StorageContext.MaxPageSize
	// - Supports grouping by any field (returns grouped results)
	List(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter) (*QueryResult, error)

	// Query executes a custom query (backend-specific)
	// - File backend: supports basic filtering/sorting
	// - Graph backend: supports Cypher queries
	// - Respects StorageContext parameters (pagination, grouping)
	Query(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query Query) (*QueryResult, error)

	// Search performs full-text search across objects
	// - Enforces permissions (read permission for object kinds)
	// - Supports fuzzy matching, stemming, and synonyms
	// - Returns matching objects with relevance scores
	// - Supports highlighting of matching text
	Search(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query SearchQuery) (*SearchResult, error)

	// BeginTransaction starts a transaction for atomic multi-object operations
	BeginTransaction(ctx context.Context) (ObjectTransaction, error)

	// BulkCreate creates multiple objects atomically
	// Returns a BulkResult with success/failure counts and per-object errors
	BulkCreate(ctx context.Context, secCtx *SecurityContext, objects []map[string]any) (*BulkResult, error)

	// BulkUpdate updates multiple objects atomically
	// Each update is {id: string, updates: map[string]any}
	// Returns a BulkResult with success/failure counts and per-object errors
	BulkUpdate(ctx context.Context, secCtx *SecurityContext, updates []BulkUpdateItem) (*BulkResult, error)

	// BulkGet retrieves multiple objects by ID
	// Returns a BulkResult with successfully retrieved objects and per-object errors
	BulkGet(ctx context.Context, secCtx *SecurityContext, ids []string) (*BulkResult, error)

	// BulkDelete deletes multiple objects atomically
	// Returns a BulkResult with success/failure counts and per-object errors
	BulkDelete(ctx context.Context, secCtx *SecurityContext, ids []string, cascade bool) (*BulkResult, error)

	// Exists checks if an object exists by ID
	// - Enforces permissions (read permission for object kind)
	// - More efficient than Read() as it doesn't load the object
	// - Returns true if object exists, false if not found
	// - Returns error only for permission or system errors
	Exists(ctx context.Context, secCtx *SecurityContext, id string) (bool, error)

	// Count counts objects matching the filter
	// - Enforces permissions (read permission for object kind)
	// - More efficient than List() as it doesn't load objects into memory
	// - Supports all ListFilter filtering capabilities
	// - Returns the count of matching objects
	Count(ctx context.Context, secCtx *SecurityContext, filter ListFilter) (int, error)

	// Aggregate performs aggregations on objects matching the filter
	// - Enforces permissions (read permission for object kind)
	// - Supports multiple aggregations in a single call
	// - Supports GroupBy via filter.GroupBy field
	// - Returns aggregated results grouped by aggregation function and group (if GroupBy specified)
	Aggregate(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error)

	// GetRelated finds objects related to the given object via reference fields
	// - relationshipType: optional filter by reference field name (e.g., "priority_plan_ref", "milestone_refs")
	//   If empty, returns all related objects regardless of relationship type
	// - depth: maximum traversal depth (1 = direct references only, 2 = references of references, etc.)
	// - Returns all related objects found up to the specified depth
	// - Enforces permissions (read permission for each related object kind)
	GetRelated(ctx context.Context, secCtx *SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error)

	// GetPath finds a path between two objects via reference relationships
	// - Returns the shortest path as a sequence of objects from fromID to toID
	// - Returns empty slice if no path exists
	// - Enforces permissions (read permission for all objects in path)
	GetPath(ctx context.Context, secCtx *SecurityContext, fromID, toID string) ([]map[string]any, error)

	// GetNeighbors finds immediate neighbors of an object (depth=1)
	// - direction: "outgoing" (objects this references), "incoming" (objects that reference this), or "both"
	// - Returns all objects directly connected via reference fields
	// - Enforces permissions (read permission for each neighbor object kind)
	GetNeighbors(ctx context.Context, secCtx *SecurityContext, id string, direction string) ([]map[string]any, error)

	// Move moves an object to a different directory/kind while preserving history
	// - newKind: The new object kind (determines new directory)
	// - Preserves created_at, created_by, and other history fields
	// - Updates hash registry atomically
	// - Creates audit event for move operation
	// - Updates references pointing to the moved object (if updateReferences is true)
	// - Returns error if object not found, new kind invalid, or move fails
	Move(ctx context.Context, secCtx *SecurityContext, id string, newKind string, updateReferences bool) error

	// Rename changes an object's ID (same kind). ITEM-851.
	// Preserves object history; updates hash registry; creates audit event.
	// If updateReferences is true, updates all objects that reference the old ID to use the new ID.
	// Returns error if object not found, new ID invalid or already exists, or rename fails.
	Rename(ctx context.Context, secCtx *SecurityContext, oldID, newID string, updateReferences bool) error

	// Shutdown gracefully shuts down background workers associated with this storage
	Shutdown(ctx context.Context) error
}

// ObjectTransaction provides transaction support for multi-object operations
type ObjectTransaction interface {
	Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error
	Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error)
	Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error
	Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// FilterOperator represents a filter operation
type FilterOperator string

const (
	// Equality operators
	OpEqual    FilterOperator = "$eq" // Equal (default if no operator specified)
	OpNotEqual FilterOperator = "$ne" // Not equal

	// Comparison operators
	OpGreaterThan        FilterOperator = "$gt"  // Greater than
	OpGreaterThanOrEqual FilterOperator = "$gte" // Greater than or equal
	OpLessThan           FilterOperator = "$lt"  // Less than
	OpLessThanOrEqual    FilterOperator = "$lte" // Less than or equal

	// Semantic date/time operators (semantic type aware)
	OpBefore     FilterOperator = "$before"     // Before (exclusive) - for timestamp/date/datetime
	OpAfter      FilterOperator = "$after"      // After (exclusive) - for timestamp/date/datetime
	OpOn         FilterOperator = "$on"         // Exactly on (equality) - for date/datetime
	OpOnOrBefore FilterOperator = "$onOrBefore" // On or before (inclusive) - for timestamp/date/datetime
	OpOnOrAfter  FilterOperator = "$onOrAfter"  // On or after (inclusive) - for timestamp/date/datetime
	OpBetween    FilterOperator = "$between"    // Between two values (inclusive) - for timestamp/date/datetime
	OpWithin     FilterOperator = "$within"     // Within a time range - for timestamp/date/datetime

	// String operators
	OpContains   FilterOperator = "$contains"   // String contains substring
	OpStartsWith FilterOperator = "$startsWith" // String starts with
	OpEndsWith   FilterOperator = "$endsWith"   // String ends with
	OpRegex      FilterOperator = "$regex"      // Regular expression match

	// Array/list operators
	OpIn     FilterOperator = "$in"     // Value is in array
	OpNotIn  FilterOperator = "$nin"    // Value is not in array
	OpHas    FilterOperator = "$has"    // Array contains value
	OpHasAll FilterOperator = "$hasAll" // Array contains all values
	OpHasAny FilterOperator = "$hasAny" // Array contains any value

	// Null/existence operators
	OpExists FilterOperator = "$exists" // Field exists (value is true/false)
	OpIsNull FilterOperator = "$isNull" // Field is null or missing
)

// FilterValue represents a filter condition
// Can be:
//   - A simple value (treated as equality, e.g., "planned" -> {"$eq": "planned"})
//   - A map with operator and value (e.g., {"$gt": 10}, {"$contains": "text"})
//   - An array for $in/$nin (e.g., {"$in": ["planned", "in_progress"]})
type FilterValue struct {
	Operator FilterOperator
	Value    any
}

// ListFilter defines filtering, sorting, pagination, and grouping options
// Filters can be specified as:
//   - Simple equality: Filters["status"] = "planned"
//   - With operator: Filters["priority"] = map[string]any{"$gt": 5}
//   - Complex: Filters["title"] = map[string]any{"$contains": "bug"}
type ListFilter struct {
	Kind    string         // Object kind (required)
	Filters map[string]any // Field -> value filters (supports operators via map syntax)
	SortBy  string         // Field to sort by (any property on the object)
	SortAsc bool           // Sort direction
	Offset  int            // Pagination offset
	Limit   int            // Pagination limit (0 = no limit, respects context max_page_size)
	GroupBy string         // Field to group by (optional, any property on the object)
	// Fields lists top-level YAML keys to return per object after materialization (hybrid projection).
	// Empty means return full maps. SortBy is merged into the projection mask automatically when Fields is non-empty.
	Fields []string
}

// Query represents a backend-specific query
type Query struct {
	Kind       string         // Object kind
	Type       QueryType      // Query type
	Expression string         // Query expression (Cypher for graph, filter for file)
	Parameters map[string]any // Query parameters
}

// QueryType represents the type of query
type QueryType string

const (
	QueryTypeFilter             QueryType = "filter"              // File backend: field filters
	QueryTypeCypher             QueryType = "cypher"              // Graph backend: Cypher query
	QueryTypeVector             QueryType = "vector"              // Graph backend: vector similarity
	QueryTypeHierarchicalVector QueryType = "hierarchical_vector" // Graph backend: macro-to-micro hierarchical vector retrieval
)

// QueryResult contains query execution results
type QueryResult struct {
	Objects []map[string]any
	Groups  map[string][]map[string]any // Grouped results (key = group value, value = objects in group)
	Meta    map[string]any
}

// AggregationFunction represents the type of aggregation
type AggregationFunction string

const (
	AggregationCount AggregationFunction = "count"
	AggregationSum   AggregationFunction = "sum"
	AggregationAvg   AggregationFunction = "avg"
	AggregationMin   AggregationFunction = "min"
	AggregationMax   AggregationFunction = "max"
)

// Aggregation defines a single aggregation operation
type Aggregation struct {
	Function AggregationFunction // count, sum, avg, min, max, group
	Field    string              // Field to aggregate (empty for count)
	Alias    string              // Result alias (e.g., "total_count", "avg_priority")
}

// AggregateResult contains aggregated query results
type AggregateResult struct {
	Aggregations map[string]any              // Alias -> aggregated value
	Groups       map[string]*AggregateResult // GroupBy results (key = group value)
	Meta         map[string]any              // Metadata (count, execution_time, etc.)
}

// SearchQuery defines a full-text search query
type SearchQuery struct {
	Query     string   // Search query string
	Kinds     []string // Object kinds to search (empty = all)
	Fields    []string // Fields to search (empty = all text fields)
	Fuzzy     bool     // Enable fuzzy matching
	Stemming  bool     // Enable stemming
	Synonyms  bool     // Enable synonym expansion
	Highlight bool     // Enable result highlighting
	Limit     int      // Maximum results
	Offset    int      // Pagination offset
	MinScore  float64  // Minimum relevance score (0.0-1.0)
}

// SearchMatch represents a single search result
type SearchMatch struct {
	Object        map[string]any      // The matching object
	Score         float64             // Relevance score (0.0-1.0)
	Highlights    map[string][]string // Field -> highlighted snippets
	MatchedFields []string            // Fields that matched
}

// SearchResult contains search results with relevance scores
type SearchResult struct {
	Objects    []SearchMatch  // Matching objects with scores
	TotalCount int            // Total number of matches
	QueryTime  int64          // Query execution time in nanoseconds
	Meta       map[string]any // Metadata
}

// BulkUpdateItem represents a single update in a bulk update operation
type BulkUpdateItem struct {
	ID      string
	Updates map[string]any
}

// BulkResult contains the results of a bulk operation
type BulkResult struct {
	SuccessCount int                  // Number of successful operations
	FailureCount int                  // Number of failed operations
	TotalCount   int                  // Total number of operations attempted
	Results      []map[string]any     // Successfully processed objects (for create/get)
	Errors       []BulkOperationError // Per-object errors
}

// BulkOperationError represents an error for a specific object in a bulk operation
type BulkOperationError struct {
	ID      string // Object ID (if available)
	Index   int    // Index in the input array
	Error   error  // The error that occurred
	Message string // Human-readable error message
}
