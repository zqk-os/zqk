package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/graph/provider"
	pkgobjects "github.com/lanceman/zqk/pkg/objects"
)

// EnsureSpecDerivedIndexes creates idempotent MemGraph indexes derived from the
// object-spec index (kind labels + Entity.id). Safe to call repeatedly.
// TRACK: REDACTED — GFS P2b.
func (g *GraphObjectStorage) EnsureSpecDerivedIndexes(ctx context.Context) error {
	if g == nil || g.conn == nil {
		return nil
	}
	_ = g.ensureEntityIDIndex(ctx)

	kinds := []string{}
	if g.projectRoot != "" {
		if idx := pkgobjects.TryLoadSpecIndexForProjectRoot(g.projectRoot); idx != nil {
			for k := range idx.Kinds {
				kinds = append(kinds, k)
			}
		}
	}
	if len(kinds) == 0 {
		kinds = []string{"backlog_item", "goal", "priority_plan", "milestone", "requirement", "criteria", "decision"}
	}

	var first error
	for _, kind := range kinds {
		label := toLabel(kind)
		if label == "" {
			continue
		}
		q := provider.Query{
			Language: provider.QueryLanguageCypher,
			Query:    fmt.Sprintf("CREATE INDEX ON :%s(id)", label),
		}
		if _, err := g.conn.ExecuteQuery(ctx, q); err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "already") || strings.Contains(msg, "exists") {
				continue
			}
			if first == nil {
				first = err
			}
		}
	}
	return first
}

// EnsureSpecDerivedIndexes acquires a pool connection and ensures indexes.
func (p *PoolAwareGraphStorage) EnsureSpecDerivedIndexes(ctx context.Context) error {
	if p == nil || p.pool == nil {
		return nil
	}
	return p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		return graphStorage.EnsureSpecDerivedIndexes(ctx)
	})
}
