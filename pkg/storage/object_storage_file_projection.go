package storage

import (
	"context"
	"fmt"

	"github.com/lanceman/zqk/pkg/config"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	pkgobjects "github.com/lanceman/zqk/pkg/objects"
)

// FileFirstProjectionStorage writes file SSOT first, then projects to an optional graph backend.
// Reads use the file SSOT. Enable via STORAGE_MODE=file+projection (brand-prefixed env).
//
// TRACK: BLI-REDACTED — audible projection errors.
// TRACK: BLI-REDACTED — RebuildProjectionFromSSOT orphan purge.
type FileFirstProjectionStorage struct {
	file       *FileObjectStorage
	projection ObjectStorageProvider
}

// NewFileFirstProjectionStorage creates a new FileFirstProjectionStorage instance.
func NewFileFirstProjectionStorage(file *FileObjectStorage, projection ObjectStorageProvider) *FileFirstProjectionStorage {
	return &FileFirstProjectionStorage{
		file:       file,
		projection: projection,
	}
}

// UnderlyingObjectStorageProvider returns the primary SSOT file storage, enabling unwrapping via UnwrapToFileObjectStorage.
func (f *FileFirstProjectionStorage) UnderlyingObjectStorageProvider() ObjectStorageProvider {
	return f.file
}

func projectionFailClosed() bool {
	v := config.SystemProjectionFailClosed().OrDefault(false)
	return v
}

func (s *FileFirstProjectionStorage) handleProjectionError(op, id string, err error) error {
	if err == nil {
		return nil
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Warn("graph projection failed after file SSOT write").
		String("op", op).
		String(pkgobjects.FieldKeyID, id).
		ErrorText(err.Error()).
		Log()
	if projectionFailClosed() {
		return errfmt.Newf("projection %s failed for %s", op, id).Wrap(err)
	}
	return nil
}

func (s *FileFirstProjectionStorage) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
	if s.projection != nil {
		id, _ := obj[pkgobjects.FieldKeyID].(string)
		if err := s.projection.Create(ctx, secCtx, obj); err != nil {
			return s.handleProjectionError("create", id, err)
		}
	}
	if err := s.file.Create(ctx, secCtx, obj); err != nil {
		return err
	}
	return nil
}

func (s *FileFirstProjectionStorage) Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error) {
	return s.file.Read(ctx, secCtx, id)
}

func (s *FileFirstProjectionStorage) Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error {
	if err := s.file.Update(ctx, secCtx, id, updates); err != nil {
		return err
	}
	if s.projection != nil {
		if exists, _ := s.projection.Exists(ctx, secCtx, id); exists {
			if err := s.projection.Update(ctx, secCtx, id, updates); err != nil {
				return s.handleProjectionError("update", id, err)
			}
		} else if fullObj, err := s.file.Read(ctx, secCtx, id); err == nil {
			if err := s.projection.Create(ctx, secCtx, fullObj); err != nil {
				return s.handleProjectionError("create", id, err)
			}
		}
	}
	return nil
}

func (s *FileFirstProjectionStorage) Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error {
	if err := s.file.Delete(ctx, secCtx, id, cascade); err != nil {
		return err
	}
	if s.projection != nil {
		if err := s.projection.Delete(ctx, secCtx, id, cascade); err != nil {
			return s.handleProjectionError("delete", id, err)
		}
	}
	return nil
}

func (s *FileFirstProjectionStorage) List(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter) (*QueryResult, error) {
	return s.file.List(ctx, secCtx, storageCtx, filter)
}

func (s *FileFirstProjectionStorage) Query(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query Query) (*QueryResult, error) {
	return s.file.Query(ctx, secCtx, storageCtx, query)
}

func (s *FileFirstProjectionStorage) Search(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query SearchQuery) (*SearchResult, error) {
	return s.file.Search(ctx, secCtx, storageCtx, query)
}

func (s *FileFirstProjectionStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	return s.file.BeginTransaction(ctx)
}

func (s *FileFirstProjectionStorage) BulkCreate(ctx context.Context, secCtx *SecurityContext, objects []map[string]any) (*BulkResult, error) {
	return s.file.BulkCreate(ctx, secCtx, objects)
}

func (s *FileFirstProjectionStorage) BulkUpdate(ctx context.Context, secCtx *SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	return s.file.BulkUpdate(ctx, secCtx, updates)
}

func (s *FileFirstProjectionStorage) BulkGet(ctx context.Context, secCtx *SecurityContext, ids []string) (*BulkResult, error) {
	return s.file.BulkGet(ctx, secCtx, ids)
}

func (s *FileFirstProjectionStorage) BulkDelete(ctx context.Context, secCtx *SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	return s.file.BulkDelete(ctx, secCtx, ids, cascade)
}

func (s *FileFirstProjectionStorage) Exists(ctx context.Context, secCtx *SecurityContext, id string) (bool, error) {
	return s.file.Exists(ctx, secCtx, id)
}

func (s *FileFirstProjectionStorage) Count(ctx context.Context, secCtx *SecurityContext, filter ListFilter) (int, error) {
	return s.file.Count(ctx, secCtx, filter)
}

func (s *FileFirstProjectionStorage) Aggregate(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	return s.file.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
}

func (s *FileFirstProjectionStorage) GetRelated(ctx context.Context, secCtx *SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	return s.file.GetRelated(ctx, secCtx, id, relationshipType, depth)
}

func (s *FileFirstProjectionStorage) GetPath(ctx context.Context, secCtx *SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return s.file.GetPath(ctx, secCtx, fromID, toID)
}

func (s *FileFirstProjectionStorage) GetNeighbors(ctx context.Context, secCtx *SecurityContext, id string, direction string) ([]map[string]any, error) {
	return s.file.GetNeighbors(ctx, secCtx, id, direction)
}

func (s *FileFirstProjectionStorage) Move(ctx context.Context, secCtx *SecurityContext, id string, newKind string, updateReferences bool) error {
	if err := s.file.Move(ctx, secCtx, id, newKind, updateReferences); err != nil {
		return err
	}
	if s.projection != nil {
		_ = s.projection.Move(ctx, secCtx, id, newKind, updateReferences)
	}
	return nil
}

func (s *FileFirstProjectionStorage) Rename(ctx context.Context, secCtx *SecurityContext, oldID, newID string, updateReferences bool) error {
	if err := s.file.Rename(ctx, secCtx, oldID, newID, updateReferences); err != nil {
		return err
	}
	if s.projection != nil {
		_ = s.projection.Rename(ctx, secCtx, oldID, newID, updateReferences)
	}
	return nil
}

func (s *FileFirstProjectionStorage) Shutdown(ctx context.Context) error {
	var err error
	if s.file != nil {
		err = s.file.Shutdown(ctx)
	}
	if s.projection != nil {
		if e := s.projection.Shutdown(ctx); e != nil {
			err = e
		}
	}
	return err
}

// GetFileSSOT returns the durability file backend.
func (s *FileFirstProjectionStorage) GetFileSSOT() *FileObjectStorage {
	return s.file
}

// GetProjection returns the optional graph projection backend.
func (s *FileFirstProjectionStorage) GetProjection() ObjectStorageProvider {
	return s.projection
}

// RebuildProjectionResult summarizes a rebuild pass.
type RebuildProjectionResult struct {
	RebuiltCount  int      `json:"rebuilt_count"`
	OrphanDeleted int      `json:"orphan_deleted"`
	Errors        []string `json:"errors,omitempty"`
}

// RebuildProjectionFromSSOT rebuilds the projection backend from objects present in file SSOT.
// It upserts every SSOT object and deletes projection orphans not present in SSOT (same kind walk).
func (s *FileFirstProjectionStorage) RebuildProjectionFromSSOT(ctx context.Context, secCtx *SecurityContext) (rebuiltCount int, err error) {
	res, err := s.RebuildProjectionFromSSOTDetailed(ctx, secCtx, false)
	if err != nil {
		return 0, err
	}
	return res.RebuiltCount, nil
}

// RebuildProjectionFromSSOTDetailed rebuilds with optional dry-run (counts only, no writes).
func (s *FileFirstProjectionStorage) RebuildProjectionFromSSOTDetailed(ctx context.Context, secCtx *SecurityContext, dryRun bool) (*RebuildProjectionResult, error) {
	out := &RebuildProjectionResult{}
	if s.projection == nil {
		return out, nil
	}

	storageCtx := pkgctx.NewStorageContext()
	kinds := s.projectionKinds()
	ssotIDs := make(map[string]struct{})

	for _, kind := range kinds {
		res, listErr := s.file.List(ctx, secCtx, storageCtx, ListFilter{Kind: kind})
		if listErr != nil || res == nil {
			if listErr != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("list %s: %v", kind, listErr))
			}
			continue
		}
		for _, obj := range res.Objects {
			id, _ := obj[pkgobjects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			ssotIDs[id] = struct{}{}
			if dryRun {
				out.RebuiltCount++
				continue
			}
			if exists, _ := s.projection.Exists(ctx, secCtx, id); exists {
				if err := s.projection.Update(ctx, secCtx, id, obj); err != nil {
					out.Errors = append(out.Errors, fmt.Sprintf("update %s: %v", id, err))
					continue
				}
			} else if err := s.projection.Create(ctx, secCtx, obj); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("create %s: %v", id, err))
				continue
			}
			out.RebuiltCount++
		}
	}

	// Orphan purge: projection nodes for walked kinds not in SSOT.
	for _, kind := range kinds {
		pres, listErr := s.projection.List(ctx, secCtx, storageCtx, ListFilter{Kind: kind})
		if listErr != nil || pres == nil {
			continue
		}
		for _, obj := range pres.Objects {
			id, _ := obj[pkgobjects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			if _, ok := ssotIDs[id]; ok {
				continue
			}
			if dryRun {
				out.OrphanDeleted++
				continue
			}
			if err := s.projection.Delete(ctx, secCtx, id, false); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("orphan delete %s: %v", id, err))
				continue
			}
			out.OrphanDeleted++
		}
	}

	if g, ok := s.projection.(*GraphObjectStorage); ok && !dryRun {
		_ = g.EnsureSpecDerivedIndexes(ctx)
	} else if p, ok := s.projection.(*PoolAwareGraphStorage); ok && !dryRun {
		_ = p.EnsureSpecDerivedIndexes(ctx)
	}

	if len(out.Errors) > 0 && projectionFailClosed() {
		return out, errfmt.Errorf("rebuild projection completed with %d errors", len(out.Errors))
	}
	return out, nil
}

func (s *FileFirstProjectionStorage) projectionKinds() []string {
	var kinds []string
	if idx := pkgobjects.TryLoadSpecIndexForProjectRoot(s.file.GetProjectRoot()); idx != nil {
		for k := range idx.Kinds {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) == 0 {
		kinds = []string{"goal", "backlog_item", "priority_plan", "milestone", "workstream", "requirement", "roadmap", "decision", "criteria"}
	}
	return kinds
}
