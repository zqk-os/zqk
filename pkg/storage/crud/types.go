package crud

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
