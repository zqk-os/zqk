package crud

import (
	"slices"

	"github.com/zqk-os/zqk/pkg/objects"
)

// QueryBuilder provides a declarative and fluent API for building ListFilter instances.
type QueryBuilder struct {
	kind    string
	filters map[string]any
	sortBy  string
	sortAsc bool
	offset  int
	limit   int
	groupBy string
	fields  []string
}

// NewQueryBuilder creates a new QueryBuilder initialized for the specified kind.
func NewQueryBuilder(kind string) *QueryBuilder {
	return &QueryBuilder{
		kind:    kind,
		filters: make(map[string]any),
		sortAsc: true,
	}
}

// Kind sets the target object kind.
func (qb *QueryBuilder) Kind(kind string) *QueryBuilder {
	qb.kind = kind
	return qb
}

// IncludeFields adds field names to the projected fields list without duplicates.
func (qb *QueryBuilder) IncludeFields(fields ...string) *QueryBuilder {
	for _, f := range fields {
		if f != "" && !slices.Contains(qb.fields, f) {
			qb.fields = append(qb.fields, f)
		}
	}
	return qb
}

// IncludeFieldsSlice adds a slice of field names to the projected fields list.
func (qb *QueryBuilder) IncludeFieldsSlice(fields []string) *QueryBuilder {
	return qb.IncludeFields(fields...)
}

// Fields is an alias for IncludeFields.
func (qb *QueryBuilder) Fields(fields ...string) *QueryBuilder {
	return qb.IncludeFields(fields...)
}

// AndFilter adds or updates a field filter constraint.
func (qb *QueryBuilder) AndFilter(field string, value any) *QueryBuilder {
	if qb.filters == nil {
		qb.filters = make(map[string]any)
	}
	qb.filters[field] = value
	return qb
}

// Filter is an alias for AndFilter.
func (qb *QueryBuilder) Filter(field string, value any) *QueryBuilder {
	return qb.AndFilter(field, value)
}

// AndFilterMap merges all key-value pairs from the given map into the builder's filters.
func (qb *QueryBuilder) AndFilterMap(filters map[string]any) *QueryBuilder {
	for k, v := range filters {
		qb.AndFilter(k, v)
	}
	return qb
}

// FilterOp sets an operator-based constraint on a field (e.g. FilterOp("created_at", "$after", cutoff)).
func (qb *QueryBuilder) FilterOp(field string, op string, value any) *QueryBuilder {
	return qb.AndFilter(field, map[string]any{op: value})
}

// OrFilter appends one or more alternative filter condition maps under the $or logical operator.
func (qb *QueryBuilder) OrFilter(conditions ...map[string]any) *QueryBuilder {
	if qb.filters == nil {
		qb.filters = make(map[string]any)
	}
	if existing, ok := qb.filters["$or"].([]map[string]any); ok {
		existing = append(existing, conditions...)
		qb.filters["$or"] = existing
		return qb
	}
	if existing, ok := qb.filters["$or"].([]any); ok {
		for _, c := range conditions {
			existing = append(existing, c)
		}
		qb.filters["$or"] = existing
		return qb
	}
	qb.filters["$or"] = conditions
	return qb
}

// OrFilterBuilders extracts filters from other QueryBuilders and appends them under the $or operator.
func (qb *QueryBuilder) OrFilterBuilders(builders ...*QueryBuilder) *QueryBuilder {
	conds := make([]map[string]any, 0, len(builders))
	for _, b := range builders {
		if b != nil {
			built := b.Build()
			if len(built.Filters) > 0 {
				conds = append(conds, built.Filters)
			}
		}
	}
	if len(conds) > 0 {
		qb.OrFilter(conds...)
	}
	return qb
}

// Status sets the status equality filter.
func (qb *QueryBuilder) Status(status string) *QueryBuilder {
	return qb.AndFilter(objects.FieldKeyStatus, status)
}

// StatusIn sets a status $in filter for any of the specified statuses.
func (qb *QueryBuilder) StatusIn(statuses ...string) *QueryBuilder {
	return qb.AndFilter(objects.FieldKeyStatus, map[string]any{
		"$in": statuses,
	})
}

// StatusNotIn sets a status $nin filter excluding the specified statuses.
func (qb *QueryBuilder) StatusNotIn(statuses ...string) *QueryBuilder {
	return qb.AndFilter(objects.FieldKeyStatus, map[string]any{
		"$nin": statuses,
	})
}

// StatusNot sets a status $ne filter.
func (qb *QueryBuilder) StatusNot(status string) *QueryBuilder {
	return qb.AndFilter(objects.FieldKeyStatus, map[string]any{
		"$ne": status,
	})
}

// Id sets the ID equality filter.
func (qb *QueryBuilder) Id(id string) *QueryBuilder {
	return qb.AndFilter(objects.FieldKeyID, id)
}

// IdIn sets an ID $in filter.
func (qb *QueryBuilder) IdIn(ids ...string) *QueryBuilder {
	return qb.AndFilter(objects.FieldKeyID, map[string]any{
		"$in": ids,
	})
}

// Limit sets the maximum number of items to return (0 = no limit).
func (qb *QueryBuilder) Limit(limit int) *QueryBuilder {
	qb.limit = limit
	return qb
}

// Offset sets the pagination offset.
func (qb *QueryBuilder) Offset(offset int) *QueryBuilder {
	qb.offset = offset
	return qb
}

// Sort sets the sort field and direction.
func (qb *QueryBuilder) Sort(sortBy string, asc bool) *QueryBuilder {
	qb.sortBy = sortBy
	qb.sortAsc = asc
	return qb
}

// SortBy sets the sort field with ascending order.
func (qb *QueryBuilder) SortBy(sortBy string) *QueryBuilder {
	return qb.Sort(sortBy, true)
}

// SortDesc sets the sort field with descending order.
func (qb *QueryBuilder) SortDesc(sortBy string) *QueryBuilder {
	return qb.Sort(sortBy, false)
}

// GroupBy sets the field to group results by.
func (qb *QueryBuilder) GroupBy(groupBy string) *QueryBuilder {
	qb.groupBy = groupBy
	return qb
}

// Clone creates a deep copy of the builder.
func (qb *QueryBuilder) Clone() *QueryBuilder {
	clone := &QueryBuilder{
		kind:    qb.kind,
		sortBy:  qb.sortBy,
		sortAsc: qb.sortAsc,
		offset:  qb.offset,
		limit:   qb.limit,
		groupBy: qb.groupBy,
	}
	if qb.filters != nil {
		clone.filters = make(map[string]any, len(qb.filters))
		for k, v := range qb.filters {
			clone.filters[k] = v
		}
	}
	if len(qb.fields) > 0 {
		clone.fields = make([]string, len(qb.fields))
		copy(clone.fields, qb.fields)
	}
	return clone
}

// Build compiles the configured state into an immutable ListFilter value.
func (qb *QueryBuilder) Build() ListFilter {
	filtersCopy := make(map[string]any, len(qb.filters))
	for k, v := range qb.filters {
		filtersCopy[k] = v
	}
	var fieldsCopy []string
	if len(qb.fields) > 0 {
		fieldsCopy = make([]string, len(qb.fields))
		copy(fieldsCopy, qb.fields)
	}

	return ListFilter{
		Kind:    qb.kind,
		Filters: filtersCopy,
		SortBy:  qb.sortBy,
		SortAsc: qb.sortAsc,
		Offset:  qb.offset,
		Limit:   qb.limit,
		GroupBy: qb.groupBy,
		Fields:  fieldsCopy,
	}
}

// ToFilter is an alias for Build for convenient inline usage.
func (qb *QueryBuilder) ToFilter() ListFilter {
	return qb.Build()
}
