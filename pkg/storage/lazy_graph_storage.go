package storage

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"path/filepath"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/graph/rpcpool"
)

// ErrGraphNotAvailable is returned when the graph backend is requested but not connected.
var ErrGraphNotAvailable = errfmt.Errorf("graph backend not available")

// LazyGraphStorage implements ObjectStorageProvider by deferring the acquisition
// of the graph connection pool until the first operation is performed.
// This prevents slow graph connections from blocking CLI initialization.
type LazyGraphStorage struct {
	provider     GraphConnectionProvider
	projectRoot  string
	useMockGraph bool

	initOnce sync.Once
	real     ObjectStorageProvider
	initErr  error
}

// NewLazyGraphStorage creates a new lazy graph storage.
func NewLazyGraphStorage(p GraphConnectionProvider, projectRoot string, useMockGraph bool) *LazyGraphStorage {
	return &LazyGraphStorage{
		provider:     p,
		projectRoot:  projectRoot,
		useMockGraph: useMockGraph,
	}
}

func (l *LazyGraphStorage) ensureInitialized(ctx context.Context) error {
	l.initOnce.Do(func() {
		var pool provider.ConnectionPool
		var err error

		if l.useMockGraph {
			mockProvider := provider.NewMockGraphProvider()
			// Use a default config for mock
			pool, err = mockProvider.CreatePool(ctx, provider.ConnectionConfig{MaxConns: 10})
		} else {
			isSchedulerDaemon := false
			if len(os.Args) >= 3 && os.Args[1] == "scheduler" && os.Args[2] == "start" {
				isSchedulerDaemon = true
			}

			if !isSchedulerDaemon {
				sockPath := filepath.Join(l.projectRoot, paths.ProjectDataDir, "scheduler", "rpcpool.sock")
				if _, statErr := fileutil.Stat(sockPath); statErr == nil {
					var rpcPool provider.ConnectionPool
					var rpcErr error
					// Retry for up to 3 seconds if connection is refused (daemon might be booting)
					for i := 0; i < 30; i++ {
						rpcPool, rpcErr = rpcpool.ConnectClient(sockPath)
						if rpcErr == nil {
							break
						}
						// If error is not connection refused, break immediately
						if !strings.Contains(rpcErr.Error(), "connection refused") && !strings.Contains(rpcErr.Error(), "no such file or directory") {
							break
						}
						time.Sleep(100 * time.Millisecond)
					}

					if rpcErr == nil {
						pool = rpcPool
					} else {
						if l.provider != nil {
							pool, err = l.provider.GetPool(ctx)
						} else {
							err = errfmt.Errorf("rpc pool connection failed and no fallback provider: %w", rpcErr)
						}
					}
				} else if l.provider != nil {
					pool, err = l.provider.GetPool(ctx)
				} else {
					err = errfmt.Errorf("no graph provider available and rpc socket not found")
				}
			} else if l.provider != nil {
				pool, err = l.provider.GetPool(ctx)
			} else {
				err = errfmt.Errorf("no graph provider available for scheduler daemon")
			}
		}

		if err != nil {
			l.initErr = err
			return
		}

		if pool != nil {
			l.real = NewPoolAwareGraphStorage(pool, l.projectRoot)
		}
	})
	return l.initErr
}

func (l *LazyGraphStorage) GetPool() provider.ConnectionPool {
	_ = l.ensureInitialized(context.Background()) // Background: request-or-shutdown derived
	if l.real == nil {
		return nil
	}
	if p, ok := l.real.(interface {
		GetPool() provider.ConnectionPool
	}); ok {
		return p.GetPool()
	}
	return nil
}

func (l *LazyGraphStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if err := l.ensureInitialized(ctx); err != nil {
		return err
	}
	if l.real == nil {
		return ErrGraphNotAvailable
	}
	return l.real.Create(ctx, secCtx, obj)
}

func (l *LazyGraphStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.Read(ctx, secCtx, id)
}

func (l *LazyGraphStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if err := l.ensureInitialized(ctx); err != nil {
		return err
	}
	if l.real == nil {
		return ErrGraphNotAvailable
	}
	return l.real.Update(ctx, secCtx, id, updates)
}

func (l *LazyGraphStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id, newKind string, updateReferences bool) error {
	if err := l.ensureInitialized(ctx); err != nil {
		return err
	}
	if l.real == nil {
		return ErrGraphNotAvailable
	}
	return l.real.Move(ctx, secCtx, id, newKind, updateReferences)
}

func (l *LazyGraphStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	if err := l.ensureInitialized(ctx); err != nil {
		return err
	}
	if l.real == nil {
		return ErrGraphNotAvailable
	}
	return l.real.Rename(ctx, secCtx, oldID, newID, updateReferences)
}

func (l *LazyGraphStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	if err := l.ensureInitialized(ctx); err != nil {
		return err
	}
	if l.real == nil {
		return ErrGraphNotAvailable
	}
	return l.real.Delete(ctx, secCtx, id, cascade)
}

func (l *LazyGraphStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.List(ctx, secCtx, storageCtx, filter)
}

func (l *LazyGraphStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.Query(ctx, secCtx, storageCtx, query)
}

func (l *LazyGraphStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return 0, err
	}
	if l.real == nil {
		return 0, ErrGraphNotAvailable
	}
	return l.real.Count(ctx, secCtx, filter)
}

func (l *LazyGraphStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
}

func (l *LazyGraphStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.Search(ctx, secCtx, storageCtx, query)
}

func (l *LazyGraphStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return false, err
	}
	if l.real == nil {
		return false, ErrGraphNotAvailable
	}
	return l.real.Exists(ctx, secCtx, id)
}

func (l *LazyGraphStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.GetRelated(ctx, secCtx, id, relationshipType, depth)
}

func (l *LazyGraphStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.GetPath(ctx, secCtx, fromID, toID)
}

func (l *LazyGraphStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.GetNeighbors(ctx, secCtx, id, direction)
}

func (l *LazyGraphStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.BulkCreate(ctx, secCtx, objects)
}

func (l *LazyGraphStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.BulkUpdate(ctx, secCtx, updates)
}

func (l *LazyGraphStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.BulkGet(ctx, secCtx, ids)
}

func (l *LazyGraphStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.BulkDelete(ctx, secCtx, ids, cascade)
}

func (l *LazyGraphStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	if err := l.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	if l.real == nil {
		return nil, ErrGraphNotAvailable
	}
	return l.real.BeginTransaction(ctx)
}

func (l *LazyGraphStorage) Shutdown(ctx context.Context) error {
	return nil
}
