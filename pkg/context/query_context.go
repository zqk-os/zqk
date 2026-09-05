package context

// SQL-style sort order strings returned by GetSortOrder.
const (
	sortOrderASC  = "ASC"
	sortOrderDESC = "DESC"
)

// QueryContext groups all query-related checks (pagination, grouping, sorting)
// Instead of scattered boolean checks throughout the codebase, this context object
// encapsulates all related logic and provides methods to check conditions all at once
type QueryContext struct {
	// Storage context settings
	MaxPageSize     int
	DefaultPageSize int
	EnableGrouping  bool
	MaxGroupSize    int

	// Query parameters
	RequestedLimit  int
	RequestedOffset int
	GroupBy         string
	SortBy          string
	SortAsc         bool

	// Computed values (set by Compute())
	effectiveLimit int
	shouldPaginate bool
	shouldGroup    bool
	shouldSort     bool
	computed       bool
}

// NewQueryContext creates a new QueryContext from storage context and filter parameters
func NewQueryContext(storageCtx *StorageContext, requestedLimit, requestedOffset int, groupBy, sortBy string, sortAsc bool) *QueryContext {
	if storageCtx == nil {
		storageCtx = NewStorageContext()
	}

	return &QueryContext{
		MaxPageSize:     storageCtx.MaxPageSize,
		DefaultPageSize: storageCtx.DefaultPageSize,
		EnableGrouping:  storageCtx.EnableGrouping,
		MaxGroupSize:    storageCtx.MaxGroupSize,
		RequestedLimit:  requestedLimit,
		RequestedOffset: requestedOffset,
		GroupBy:         groupBy,
		SortBy:          sortBy,
		SortAsc:         sortAsc,
		computed:        false,
	}
}

// Compute calculates all derived values based on the context settings
// This groups all the scattered boolean checks into a single method
func (q *QueryContext) Compute() {
	if q.computed {
		return
	}

	// Compute effective limit (groups all pagination limit checks)
	q.effectiveLimit = q.RequestedLimit
	if q.MaxPageSize > 0 {
		if q.effectiveLimit == 0 || q.effectiveLimit > q.MaxPageSize {
			q.effectiveLimit = q.MaxPageSize
		}
	}
	if q.effectiveLimit == 0 && q.DefaultPageSize > 0 {
		q.effectiveLimit = q.DefaultPageSize
	}

	// Compute pagination flag (groups all pagination checks)
	q.shouldPaginate = q.effectiveLimit > 0 || q.RequestedOffset > 0

	// Compute grouping flag (groups all grouping checks)
	q.shouldGroup = q.GroupBy != emptyContextValue && q.EnableGrouping

	// Compute sorting flag
	q.shouldSort = q.SortBy != emptyContextValue

	q.computed = true
}

// GetEffectiveLimit returns the effective limit after applying context constraints
func (q *QueryContext) GetEffectiveLimit() int {
	q.Compute()
	return q.effectiveLimit
}

// ShouldPaginate returns whether pagination should be applied
func (q *QueryContext) ShouldPaginate() bool {
	q.Compute()
	return q.shouldPaginate
}

// ShouldGroup returns whether grouping should be applied
func (q *QueryContext) ShouldGroup() bool {
	q.Compute()
	return q.shouldGroup
}

// ShouldSort returns whether sorting should be applied
func (q *QueryContext) ShouldSort() bool {
	return q.SortBy != emptyContextValue
}

// GetSortOrder returns the sort order string ("ASC" or "DESC")
func (q *QueryContext) GetSortOrder() string {
	if q.SortAsc {
		return sortOrderASC
	}
	return sortOrderDESC
}
