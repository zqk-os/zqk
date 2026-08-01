package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	pkgobjects "github.com/lanceman/zqk/pkg/objects"
)

// FileFirstProjectionStorage writes file SSOT first, then projects to an optional graph backend.
// Reads use the file SSOT. Enable via STORAGE_MODE=file+projection (brand-prefixed env).
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

func (s *FileFirstProjectionStorage) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
	if err := s.file.Create(ctx, secCtx, obj); err != nil {
		return err
	}
	if s.projection != nil {
		_ = s.projection.Create(ctx, secCtx, obj)
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
			_ = s.projection.Update(ctx, secCtx, id, updates)
		} else {
			if fullObj, err := s.file.Read(ctx, secCtx, id); err == nil {
				_ = s.projection.Create(ctx, secCtx, fullObj)
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
		_ = s.projection.Delete(ctx, secCtx, id, cascade)
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
	res, err := s.file.BulkCreate(ctx, secCtx, objects)
	if err == nil && s.projection != nil {
		_, _ = s.projection.BulkCreate(ctx, secCtx, objects)
	}
	return res, err
}

func (s *FileFirstProjectionStorage) BulkUpdate(ctx context.Context, secCtx *SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	res, err := s.file.BulkUpdate(ctx, secCtx, updates)
	if err == nil && s.projection != nil {
		_, _ = s.projection.BulkUpdate(ctx, secCtx, updates)
	}
	return res, err
}

func (s *FileFirstProjectionStorage) BulkGet(ctx context.Context, secCtx *SecurityContext, ids []string) (*BulkResult, error) {
	return s.file.BulkGet(ctx, secCtx, ids)
}

func (s *FileFirstProjectionStorage) BulkDelete(ctx context.Context, secCtx *SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	res, err := s.file.BulkDelete(ctx, secCtx, ids, cascade)
	if err == nil && s.projection != nil {
		_, _ = s.projection.BulkDelete(ctx, secCtx, ids, cascade)
	}
	return res, err
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

func (s *FileFirstProjectionStorage) GetFileSSOT() *FileObjectStorage {
	return s.file
}

func (s *FileFirstProjectionStorage) GetProjection() ObjectStorageProvider {
	return s.projection
}

// RebuildProjectionFromSSOT rebuilds the projection backend from objects present in file SSOT.
func (s *FileFirstProjectionStorage) RebuildProjectionFromSSOT(ctx context.Context, secCtx *SecurityContext) (rebuiltCount int, err error) {
	if s.projection == nil {
		return 0, nil
	}

	count := 0
	storageCtx := pkgctx.NewStorageContext()

	var kinds []string
	if idx := pkgobjects.TryLoadSpecIndexForProjectRoot(s.file.GetProjectRoot()); idx != nil {
		for k := range idx.Kinds {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) == 0 {
		kinds = []string{"goal", "backlog_item", "priority_plan", "milestone", "workstream", "requirement", "roadmap", "decision"}
	}

	for _, kind := range kinds {
		res, err := s.file.List(ctx, secCtx, storageCtx, ListFilter{Kind: kind})
		if err != nil || res == nil {
			continue
		}
		for _, obj := range res.Objects {
			id, _ := obj[pkgobjects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			if exists, _ := s.projection.Exists(ctx, secCtx, id); exists {
				if err := s.projection.Update(ctx, secCtx, id, obj); err == nil {
					count++
				}
			} else {
				if err := s.projection.Create(ctx, secCtx, obj); err == nil {
					count++
				}
			}
		}
	}

	return count, nil
}
