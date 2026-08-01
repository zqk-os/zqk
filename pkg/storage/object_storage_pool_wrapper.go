package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	// PoolAwareGraphStorage wraps GraphObjectStorage to use a connection pool
	// for each operation. This allows GraphObjectStorage to work with
	// the connection pool pattern used by the graph backend.
)

type PoolAwareGraphStorage struct {
	pool        provider.ConnectionPool
	projectRoot string // workspace root for spec resolution (FileSpecStorage / SpecLoader)
}

// NewPoolAwareGraphStorage creates a new pool-aware graph storage wrapper.
// projectRoot should be the zqk project root so object specs load from the same tree as the file backend.
func NewPoolAwareGraphStorage(pool provider.ConnectionPool, projectRoot string) *PoolAwareGraphStorage {
	return &PoolAwareGraphStorage{
		pool:        pool,
		projectRoot: projectRoot,
	}
}

func (p *PoolAwareGraphStorage) GetPool() provider.ConnectionPool {
	return p.pool
}

// Create creates a new object using a connection from the pool
func (p *PoolAwareGraphStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	var result error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result = graphStorage.Create(ctx, secCtx, obj)
		return result
	})
	if err != nil {
		return err
	}
	return result
}

// Read retrieves an object by ID using a connection from the pool
func (p *PoolAwareGraphStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	var result map[string]any
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.Read(ctx, secCtx, id)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// Update updates an existing object using a connection from the pool
func (p *PoolAwareGraphStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	var result error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result = graphStorage.Update(ctx, secCtx, id, updates)
		return result
	})
	if err != nil {
		return err
	}
	return result
}

// Move moves an object to a different directory/kind using a connection from the pool
func (p *PoolAwareGraphStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id, newKind string, updateReferences bool) error {
	var result error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result = graphStorage.Move(ctx, secCtx, id, newKind, updateReferences)
		return result
	})
	if err != nil {
		return err
	}
	return result
}

// Rename changes an object's ID (same kind). Delegates to graph storage (returns error for graph backend).
func (p *PoolAwareGraphStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	var result error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result = graphStorage.Rename(ctx, secCtx, oldID, newID, updateReferences)
		return result
	})
	if err != nil {
		return err
	}
	return result
}

// Delete deletes an object using a connection from the pool
func (p *PoolAwareGraphStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	var result error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result = graphStorage.Delete(ctx, secCtx, id, cascade)
		return result
	})
	if err != nil {
		return err
	}
	return result
}

// List lists objects using a connection from the pool
//
//nolint:gocritic // Interface requires value semantics for ListFilter
func (p *PoolAwareGraphStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	var result *QueryResult
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.List(ctx, secCtx, storageCtx, filter)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// Query executes a custom query using a connection from the pool
func (p *PoolAwareGraphStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	var result *QueryResult
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.Query(ctx, secCtx, storageCtx, query)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// BeginTransaction starts a transaction using a connection from the pool
func (p *PoolAwareGraphStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	// Get a connection from the pool for the transaction
	conn, err := p.pool.GetConnection(ctx)
	if err != nil {
		return nil, err
	}

	graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
	if err != nil {
		var _err_83553120 = p.pool.ReturnConnection(conn)
		if _err_83553120 != nil {
			logging.Fluent(

				// Start transaction on the graph storage
				logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83553120).Log()
		}
		return nil, err
	}

	tx, err := graphStorage.BeginTransaction(ctx)
	//nolint:errcheck // Connection cleanup - error acceptable
	if err != nil {
		var _err_83553406 = p.pool.ReturnConnection(conn)
		if _err_83553406 != nil {
			logging.Fluent(

				// Return a transaction wrapper that returns the connection when done
				logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83553406).Log()
		}
		return nil, err
	}

	return &poolAwareTransaction{
		tx:   tx,
		conn: conn,
		pool: p.pool,
	}, nil
}

// poolAwareTransaction wraps a transaction and manages connection return
type poolAwareTransaction struct {
	tx   ObjectTransaction
	conn provider.GraphConnection
	pool provider.ConnectionPool
}

func (pt *poolAwareTransaction) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return pt.tx.Create(ctx, secCtx, obj)
}

func (pt *poolAwareTransaction) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return pt.tx.Read(ctx, secCtx, id)
}

func (pt *poolAwareTransaction) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return pt.tx.Update(ctx, secCtx, id, updates)
}

func (pt *poolAwareTransaction) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	return pt.tx.Delete(ctx, secCtx, id, cascade)
}

//nolint:errcheck // Connection cleanup - error acceptable
func (pt *poolAwareTransaction) Commit(ctx context.Context) error {
	err := pt.tx.Commit(ctx)
	var _err_83554631 = pt.pool.ReturnConnection(pt.conn)
	if _err_83554631 !=

		//nolint:errcheck // Connection cleanup - error acceptable
		nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83554631).Log()
	}
	return err
}

func (pt *poolAwareTransaction) Rollback(ctx context.Context) error {
	//nolint:errcheck // Connection cleanup - error acceptable
	err := pt.tx.Rollback(ctx)
	var _err_83554841 = pt.pool.ReturnConnection(pt.conn)
	if _err_83554841 !=

		// BulkCreate creates multiple objects atomically using a connection from the pool
		nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83554841).Log()
	}
	return err
}

func (p *PoolAwareGraphStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	var result *BulkResult
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.BulkCreate(ctx, secCtx, objects)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// BulkUpdate updates multiple objects atomically using a connection from the pool
func (p *PoolAwareGraphStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	var result *BulkResult
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.BulkUpdate(ctx, secCtx, updates)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// BulkGet retrieves multiple objects by ID using a connection from the pool
func (p *PoolAwareGraphStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	var result *BulkResult
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.BulkGet(ctx, secCtx, ids)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// BulkDelete deletes multiple objects atomically using a connection from the pool
func (p *PoolAwareGraphStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	var result *BulkResult
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.BulkDelete(ctx, secCtx, ids, cascade)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// Exists checks if an object exists using a connection from the pool
func (p *PoolAwareGraphStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	var result bool
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.Exists(ctx, secCtx, id)
		return resultErr
	})
	if err != nil {
		return false, err
	}
	return result, resultErr
}

// Count counts objects matching the filter using a connection from the pool
//
//nolint:gocritic // Interface requires value semantics for ListFilter
func (p *PoolAwareGraphStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	var result int
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.Count(ctx, secCtx, filter)
		return resultErr
	})
	if err != nil {
		return 0, err
	}
	return result, resultErr
}

// GetRelated finds objects related to the given object using a connection from the pool
func (p *PoolAwareGraphStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id, relationshipType string, depth int) ([]map[string]any, error) {
	var result []map[string]any
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.GetRelated(ctx, secCtx, id, relationshipType, depth)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// GetPath finds a path between two objects using a connection from the pool
func (p *PoolAwareGraphStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	var result []map[string]any
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.GetPath(ctx, secCtx, fromID, toID)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// GetNeighbors finds immediate neighbors of an object using a connection from the pool
func (p *PoolAwareGraphStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id, direction string) ([]map[string]any, error) {
	var result []map[string]any
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.GetNeighbors(ctx, secCtx, id, direction)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// Aggregate performs aggregations on objects using a connection from the pool
//
//nolint:gocritic // Interface requires value semantics for ListFilter
func (p *PoolAwareGraphStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	var result *AggregateResult
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

// Search performs full-text search using a connection from the pool
//
//nolint:gocritic // Interface requires value semantics for SearchQuery
func (p *PoolAwareGraphStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	var result *SearchResult
	var resultErr error
	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		graphStorage, err := NewGraphObjectStorage(conn, p.projectRoot)
		if err != nil {
			return err
		}
		result, resultErr = graphStorage.Search(ctx, secCtx, storageCtx, query)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

func (p *PoolAwareGraphStorage) Shutdown(ctx context.Context) error {
	return nil
}
