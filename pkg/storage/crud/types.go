package crud

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

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

// Build returns self (identity), enabling ListFilter to be used wherever a builder's Build() is called.
func (lf ListFilter) Build() ListFilter {
	return lf
}

// ToFilter returns self.
func (lf ListFilter) ToFilter() ListFilter {
	return lf
}

// IncludeFields returns a copy of ListFilter with Fields set.
func (lf ListFilter) IncludeFields(fields ...string) ListFilter {
	return lf.WithFields(fields...)
}

// AndFilter returns a copy of ListFilter with an additional filter condition.
func (lf ListFilter) AndFilter(field string, value any) ListFilter {
	return lf.WithFilter(field, value)
}

// Status returns a copy of ListFilter with status set.
func (lf ListFilter) Status(status string) ListFilter {
	return lf.WithFilter(objects.FieldKeyStatus, status)
}

// StatusIn returns a copy of ListFilter with status in statuses.
func (lf ListFilter) StatusIn(statuses ...string) ListFilter {
	vals := make([]any, len(statuses))
	for i, s := range statuses {
		vals[i] = s
	}
	return lf.WithFilter(objects.FieldKeyStatus, map[string]any{"$in": vals})
}

// StatusNotIn returns a copy of ListFilter with status not in statuses.
func (lf ListFilter) StatusNotIn(statuses ...string) ListFilter {
	vals := make([]any, len(statuses))
	for i, s := range statuses {
		vals[i] = s
	}
	return lf.WithFilter(objects.FieldKeyStatus, map[string]any{"$nin": vals})
}

// StatusNot returns a copy of ListFilter with status != status.
func (lf ListFilter) StatusNot(status string) ListFilter {
	return lf.WithFilter(objects.FieldKeyStatus, map[string]any{"$ne": status})
}

// FilterOp returns a copy of ListFilter with an operator condition.
func (lf ListFilter) FilterOp(field, op string, val any) ListFilter {
	return lf.WithFilter(field, map[string]any{op: val})
}

// Id returns a copy of ListFilter with ID filter set.
func (lf ListFilter) Id(id string) ListFilter {
	return lf.WithFilter(objects.FieldKeyID, id)
}

// WithLimit returns a copy of ListFilter with Limit set.
func (lf ListFilter) WithLimit(limit int) ListFilter {
	lf.Limit = limit
	return lf
}

// WithFields returns a copy of ListFilter with Fields set.
func (lf ListFilter) WithFields(fields ...string) ListFilter {
	lf.Fields = append([]string(nil), fields...)
	return lf
}

// WithFilter returns a copy of ListFilter with an additional filter condition.
func (lf ListFilter) WithFilter(field string, value any) ListFilter {
	m := make(map[string]any, len(lf.Filters)+1)
	for k, v := range lf.Filters {
		m[k] = v
	}
	m[field] = value
	lf.Filters = m
	return lf
}

// WithSort returns a copy of ListFilter with sorting options set.
func (lf ListFilter) WithSort(sortBy string, asc bool) ListFilter {
	lf.SortBy = sortBy
	lf.SortAsc = asc
	return lf
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
