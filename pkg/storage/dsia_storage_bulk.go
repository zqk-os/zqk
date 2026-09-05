// Extracted from dsia_storage.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"

	"github.com/lanceman/zqk/pkg/objects"
)

func (p *DSIAStorageProvider) BulkCreate(ctx context.Context, secCtx *SecurityContext, objs []map[string]any) (*BulkResult, error) {
	result := &BulkResult{
		TotalCount: len(objs),
		Results:    make([]map[string]any, 0, len(objs)),
		Errors:     make([]BulkOperationError, 0),
	}

	for i, obj := range objs {
		id, _ := obj[objects.FieldKeyID].(string)
		err := p.Create(ctx, secCtx, obj)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      id,
				Index:   i,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			result.SuccessCount++
			result.Results = append(result.Results, obj)
		}
	}

	return result, nil
}

func (p *DSIAStorageProvider) BulkUpdate(ctx context.Context, secCtx *SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	result := &BulkResult{
		TotalCount: len(updates),
		Results:    make([]map[string]any, 0, len(updates)),
		Errors:     make([]BulkOperationError, 0),
	}

	for i, updateItem := range updates {
		err := p.Update(ctx, secCtx, updateItem.ID, updateItem.Updates)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      updateItem.ID,
				Index:   i,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			result.SuccessCount++
			result.Results = append(result.Results, updateItem.Updates)
		}
	}

	return result, nil
}

func (p *DSIAStorageProvider) BulkGet(ctx context.Context, secCtx *SecurityContext, ids []string) (*BulkResult, error) {
	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0, len(ids)),
		Errors:     make([]BulkOperationError, 0),
	}

	for i, id := range ids {
		obj, err := p.Read(ctx, secCtx, id)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      id,
				Index:   i,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			result.SuccessCount++
			result.Results = append(result.Results, obj)
		}
	}

	return result, nil
}

func (p *DSIAStorageProvider) BulkDelete(ctx context.Context, secCtx *SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	for i, id := range ids {
		err := p.Delete(ctx, secCtx, id, cascade)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      id,
				Index:   i,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			result.SuccessCount++
			result.Results = append(result.Results, map[string]any{objects.FieldKeyID: id, "deleted": true})
		}
	}

	return result, nil
}
