package storage

import "github.com/lanceman/zqk/pkg/graph/provider"

// NewMockGraphConnectionForAggregateWithQueryResult returns a [provider.GraphConnection] backed by
// the same mock as [newMockGraphConnectionForAggregate], with one query string pre-mapped to a result.
func NewMockGraphConnectionForAggregateWithQueryResult(query string, result *provider.QueryResult) provider.GraphConnection {
	m := newMockGraphConnectionForAggregate()
	m.queryResults[query] = result
	return m
}
