package storage

// Facade boundary for upcoming decomposition.

import (
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/storage/graph"
)

type GraphLock = graph.GraphLock

func NewGraphLock(pool provider.ConnectionPool, resourceID, ownerID string) *GraphLock {
	return graph.NewGraphLock(pool, resourceID, ownerID)
}
