package audit

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// ListQuery is the audit-side List shape. It does not import storage.ListFilter.
type ListQuery struct {
	Kind    string
	Filters map[string]any
	SortBy  string
	SortAsc bool
	Limit   int
}

// ObjectQuery lists objects for aggregation ingest and window lookup.
// Implementations live in package storage so this package stays acyclic.
type ObjectQuery interface {
	List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, q ListQuery) ([]map[string]any, error)
}

// MetricStore creates and updates aggregation metrics. EventStore is Create-only
// (flush must not use it); Update is the upsert half of window uniqueness.
type MetricStore interface {
	EventStore
	Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
}

// EventsOldestFirst is a created_at ascending List query (window ingest, age scans).
func EventsOldestFirst(kind string, filters map[string]any, limit int) ListQuery {
	return ListQuery{
		Kind:    kind,
		Filters: filters,
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   limit,
	}
}

// EventsNewestFirst is a created_at descending List query (overlap fallback).
func EventsNewestFirst(kind string, filters map[string]any, limit int) ListQuery {
	return ListQuery{
		Kind:    kind,
		Filters: filters,
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: false,
		Limit:   limit,
	}
}

// FirstMatch is an unordered List query capped at one row (exact window lookup).
func FirstMatch(kind string, filters map[string]any) ListQuery {
	return ListQuery{Kind: kind, Filters: filters, Limit: 1}
}

// Limited is an unordered List query with an explicit row cap (id-in status filter).
func Limited(kind string, filters map[string]any, limit int) ListQuery {
	return ListQuery{Kind: kind, Filters: filters, Limit: limit}
}

// FirstObject returns objs[0] or nil.
func FirstObject(objs []map[string]any) map[string]any {
	if len(objs) == 0 {
		return nil
	}
	return objs[0]
}
