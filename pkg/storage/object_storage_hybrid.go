package storage

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	pkgobjects "github.com/zqk-os/zqk/pkg/objects"
	// HybridObjectStorage wraps two providers: primary and secondary.
	// Writes are performed on both (primary first), while reads are performed on primary only.
)

type HybridObjectStorage struct {
	primary   ObjectStorageProvider
	secondary ObjectStorageProvider
	locks     []*sync.RWMutex
}

// NewHybridObjectStorage creates a new HybridObjectStorage
func NewHybridObjectStorage(primary, secondary ObjectStorageProvider) *HybridObjectStorage {
	locks := make([]*sync.RWMutex, 256)
	for i := range locks {
		locks[i] = &sync.RWMutex{}
	}
	return &HybridObjectStorage{
		primary:   primary,
		secondary: secondary,
		locks:     locks,
	}
}

func (h *HybridObjectStorage) getLockIndex(id string) int {
	if id == "" {
		return 0
	}
	var hash uint32
	for i := 0; i < len(id); i++ {
		hash = hash*31 + uint32(id[i])
	}
	return int(hash % 256)
}

func (h *HybridObjectStorage) getLock(id string) *sync.RWMutex {
	return h.locks[h.getLockIndex(id)]
}

func (h *HybridObjectStorage) withLock(id string, op func() error) error {
	if id == "" {
		return op()
	}
	mu := h.getLock(id)
	mu.Lock()
	defer mu.Unlock()
	return op()
}

func (h *HybridObjectStorage) withRLock(id string, op func() (map[string]any, error)) (map[string]any, error) {
	if id == "" {
		return op()
	}
	mu := h.getLock(id)
	mu.RLock()
	defer mu.RUnlock()
	return op()
}

func (h *HybridObjectStorage) withLocks(id1, id2 string, op func() error) error {
	idx1 := h.getLockIndex(id1)
	idx2 := h.getLockIndex(id2)

	if idx1 == idx2 {
		mu := h.locks[idx1]
		mu.Lock()
		defer mu.Unlock()
		return op()
	}

	if idx1 > idx2 {
		idx1, idx2 = idx2, idx1
	}

	h.locks[idx1].Lock()
	defer h.locks[idx1].Unlock()

	h.locks[idx2].Lock()
	defer h.locks[idx2].Unlock()

	return op()
}

func (h *HybridObjectStorage) withBulkLocks(ids []string, op func() error) error {
	if len(ids) == 0 {
		return op()
	}
	indicesMap := make(map[int]struct{})
	for _, id := range ids {
		if id != "" {
			indicesMap[h.getLockIndex(id)] = struct{}{}
		}
	}
	var indices []int
	for idx := range indicesMap {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	for _, idx := range indices {
		h.locks[idx].Lock()
	}

	// Unlocking in reverse order is technically best practice though not strictly necessary for simple mutexes.
	defer func() {
		for i := len(indices) - 1; i >= 0; i-- {
			h.locks[indices[i]].Unlock()
		}
	}()
	return op()
}

// Create creates a new object in both providers
func (h *HybridObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	var objID string
	if v, ok := obj[pkgobjects.FieldKeyID].(string); ok {
		objID = v
	}
	return h.withLock(objID, func() error {
		if err := h.primary.Create(ctx, secCtx, obj); err != nil {
			return err
		}

		// Mirror to secondary
		if err := h.secondary.Create(ctx, secCtx, obj); err != nil {
			// If secondary fails, it's a major issue for git-traceability
			return errfmt.Newf(ConstStreamFailedToMirrorCreateToSecondaryStorage).Wrap(err)
		}

		return nil
	})
}

// Read retrieves an object by ID from the primary provider with fallback to secondary
func (h *HybridObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return h.withRLock(id, func() (map[string]any, error) {
		obj, err := h.primary.Read(ctx, secCtx, id)
		if err == nil {
			return obj, nil
		}

		// Fallback to secondary if not found in primary or if primary is not available
		// (useful during migration or in environments without graph backend)
		if errors.Is(err, ErrObjectNotFound) || errors.Is(err, ErrGraphNotAvailable) ||
			strings.Contains(err.Error(), ErrObjectNotFound.Error()) || strings.Contains(err.Error(), ErrGraphNotAvailable.Error()) {
			return h.secondary.Read(ctx, secCtx, id)
		}

		return nil, err
	})
}

// Update updates an existing object in both providers.
// If the object is missing from the primary provider but exists in the secondary,
// it performs a 'lazy migration' by creating it in the primary first.
func (h *HybridObjectStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return h.withLock(id, func() error {
		existsInPrimary, err := h.primary.Exists(ctx, secCtx, id)
		if err != nil {
			return err
		}

		if !existsInPrimary {
			// Check secondary
			existsInSecondary, err := h.secondary.Exists(ctx, secCtx, id)
			if err != nil {
				return err
			}

			if existsInSecondary {
				// Perform lazy migration: ingest from secondary to primary
				obj, err := h.secondary.Read(ctx, secCtx, id)
				if err != nil {
					return err
				}

				// Merge updates into the object before creating in primary
				// This avoids double-write and ensures the primary starts with correct data
				for k, v := range updates {
					obj[k] = v
				}

				// Create in primary
				if err := h.primary.Create(ctx, secCtx, obj); err != nil {
					return errfmt.Newf(ConstStreamLazyMigrationFailedDuringPrimaryCreateForStr, id).Wrap(err)
				}

				// Update secondary as usual
				if err := h.secondary.Update(ctx, secCtx, id, updates); err != nil {
					if sErr := handleSecondaryUpdateError(id, err); sErr != nil {
						return sErr
					}
				}

				return nil
			}
		}

		// Normal hybrid update (both exist or handled by primary error)
		if err := h.primary.Update(ctx, secCtx, id, updates); err != nil {
			return err
		}

		if err := h.secondary.Update(ctx, secCtx, id, updates); err != nil {
			if sErr := handleSecondaryUpdateError(id, err); sErr != nil {
				return sErr
			}
		}

		return nil
	})
}

// Delete deletes an object from both providers
func (h *HybridObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	return h.withLock(id, func() error {
		primaryErr := h.primary.Delete(ctx, secCtx, id, cascade)
		if primaryErr != nil {
			isNotFound := errors.Is(primaryErr, ErrObjectNotFound) ||
				strings.Contains(primaryErr.Error(), "object not found") ||
				strings.Contains(primaryErr.Error(), "not found") ||
				strings.Contains(primaryErr.Error(), "does not exist")
			if !isNotFound {
				return primaryErr
			}
		}

		// Mirror to secondary
		secondaryErr := h.secondary.Delete(ctx, secCtx, id, cascade)
		if secondaryErr != nil {
			if !strings.Contains(secondaryErr.Error(), "object not found") && !errors.Is(secondaryErr, ErrObjectNotFound) {
				return errfmt.Newf(ConstStreamFailedToMirrorDeleteToSecondaryStorage).Wrap(secondaryErr)
			}
		}

		return nil
	})
}

// List lists objects using the primary provider, with optimization for high-volume kinds
func (h *HybridObjectStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	// Optimization: For high-volume kinds, prefer the secondary (file/stream) storage if it supports fast listing.
	// This prevents massive graph scans from hanging the CLI.
	if IsHighVolumeKindForCache(filter.Kind) {
		return h.secondary.List(ctx, secCtx, storageCtx, filter)
	}
	return h.primary.List(ctx, secCtx, storageCtx, filter)
}

// Query executes a query on the primary provider
func (h *HybridObjectStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	// Optimization: For high-volume kinds in simple filter queries, prefer secondary storage.
	if query.Type == QueryTypeFilter && IsHighVolumeKindForCache(query.Kind) {
		return h.secondary.Query(ctx, secCtx, storageCtx, query)
	}
	return h.primary.Query(ctx, secCtx, storageCtx, query)
}

// Search performs full-text search using the primary provider
func (h *HybridObjectStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	return h.primary.Search(ctx, secCtx, storageCtx, query)
}

// BeginTransaction starts a transaction that wraps transactions from both providers.

func (h *HybridObjectStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	exists, err := h.primary.Exists(ctx, secCtx, id)
	if err == nil && exists {
		return true, nil
	}

	// If primary is not available, check secondary
	if err != nil && (errors.Is(err, ErrGraphNotAvailable) || strings.Contains(err.Error(), ErrGraphNotAvailable.Error())) {
		return h.secondary.Exists(ctx, secCtx, id)
	}

	// Fallback to secondary if not found in primary
	return h.secondary.Exists(ctx, secCtx, id)
}

// Count counts objects using the primary provider, with optimization for high-volume kinds
func (h *HybridObjectStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	// Optimization: For high-volume kinds, prefer the secondary (file/stream) storage.
	if IsHighVolumeKindForCache(filter.Kind) {
		return h.secondary.Count(ctx, secCtx, filter)
	}
	return h.primary.Count(ctx, secCtx, filter)
}

// Aggregate performs aggregations using the primary provider
func (h *HybridObjectStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	return h.primary.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
}

// GetRelated finds related objects using the primary provider with fallback to secondary
func (h *HybridObjectStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	results, err := h.primary.GetRelated(ctx, secCtx, id, relationshipType, depth)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	// Fallback to secondary if primary fails or returns no results (useful during migration)
	return h.secondary.GetRelated(ctx, secCtx, id, relationshipType, depth)
}

// GetPath finds a path using the primary provider with fallback to secondary
func (h *HybridObjectStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	results, err := h.primary.GetPath(ctx, secCtx, fromID, toID)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	// Fallback to secondary
	return h.secondary.GetPath(ctx, secCtx, fromID, toID)
}

// GetNeighbors finds neighbors using the primary provider with fallback to secondary
func (h *HybridObjectStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	results, err := h.primary.GetNeighbors(ctx, secCtx, id, direction)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	// Fallback to secondary
	return h.secondary.GetNeighbors(ctx, secCtx, id, direction)
}

// Move moves an object in both providers
func (h *HybridObjectStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, newKind string, updateReferences bool) error {
	return h.withLock(id, func() error {
		if err := h.primary.Move(ctx, secCtx, id, newKind, updateReferences); err != nil {
			return err
		}

		// Mirror to secondary
		if err := h.secondary.Move(ctx, secCtx, id, newKind, updateReferences); err != nil {
			return errfmt.Newf(ConstStreamFailedToMirrorMoveToSecondaryStorage).Wrap(err)
		}

		return nil
	})
}

// Rename renames an object in both providers
func (h *HybridObjectStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	return h.withLocks(oldID, newID, func() error {
		if err := h.primary.Rename(ctx, secCtx, oldID, newID, updateReferences); err != nil {
			return err
		}

		// Mirror to secondary
		if err := h.secondary.Rename(ctx, secCtx, oldID, newID, updateReferences); err != nil {
			return errfmt.Newf(ConstStreamFailedToMirrorRenameToSecondaryStorage).Wrap(err)
		}

		return nil
	})
}

// GetPrimary returns the primary storage provider
func (h *HybridObjectStorage) GetPrimary() ObjectStorageProvider {
	return h.primary
}

// GetSecondary returns the secondary storage provider
func (h *HybridObjectStorage) GetSecondary() ObjectStorageProvider {
	return h.secondary
}

func (h *HybridObjectStorage) Shutdown(ctx context.Context) error {
	var err error
	if h.primary != nil {
		err = h.primary.Shutdown(ctx)
	}
	if h.secondary != nil {
		if e := h.secondary.Shutdown(ctx); e != nil {
			err = e
		}
	}
	return err
}

func handleSecondaryUpdateError(id string, err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, "validation failed") || !strings.Contains(errStr, "object not found") {
		return errfmt.Newf(ConstStreamFailedToMirrorUpdateToSecondaryStorageForStr, id).Wrap(err)
	}
	return nil
}
