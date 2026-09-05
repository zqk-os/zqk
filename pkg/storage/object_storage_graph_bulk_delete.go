// Extracted from object_storage_graph_bulk.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

func (g *GraphObjectStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
	}

	if !IsCLIOperation(ctx, secCtx) {
		return nil, errfmt.Errorf(ConstStreamDeleteOperationsMustBePerformedThroughCli)
	}

	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0, len(ids)),
		Errors:     make([]BulkOperationError, 0),
	}
	if len(ids) == 0 {
		return result, nil
	}

	if cascade {
		return g.bulkDeleteCascadePerID(ctx, secCtx, ids, result)
	}

	if err := g.idValidator.LoadPatterns(); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn("bulk delete: id pattern load failed; continuing with inferred kinds where possible").
			WithError(err).
			Log()
	}

	leafGraph := NewDependencyGraph()
	leafIDs := make([]string, 0, len(ids))
	nonLeafIDs := make([]string, 0)
	idIndex := make(map[string]int, len(ids))
	for i, id := range ids {
		idIndex[id] = i
		kind := g.idValidator.InferKindFromID(id)
		if kind != emptyValue && leafGraph.IsLeafNode(kind) {
			if err := g.checkPermission(secCtx, OpDelete, kind); err != nil {
				result.FailureCount++
				result.Errors = append(result.Errors, BulkOperationError{
					ID: id, Index: i, Error: err,
					Message: fmt.Sprintf("permission denied for delete: %v", err),
				})
				continue
			}
			leafIDs = append(leafIDs, id)
			continue
		}
		nonLeafIDs = append(nonLeafIDs, id)
	}

	if len(leafIDs) > 0 {
		_ = g.ensureEntityIDIndex(ctx)
		if err := g.bulkDetachDeleteByIDs(ctx, leafIDs, idIndex, result); err != nil {
			return result, err
		}
	}
	if len(nonLeafIDs) > 0 {
		if err := g.bulkDeleteNonLeafLegacy(ctx, secCtx, nonLeafIDs, idIndex, result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (g *GraphObjectStorage) bulkDeleteCascadePerID(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, result *BulkResult) (*BulkResult, error) {
	for i, id := range ids {
		if err := g.Delete(ctx, secCtx, id, true); err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID: id, Index: i, Error: err,
				Message: fmt.Sprintf(ConstStreamFailedToDeleteObjectStrVal, id, err),
			})
			continue
		}
		result.SuccessCount++
		result.Results = append(result.Results, map[string]any{objects.FieldKeyID: id})
	}
	return result, nil
}

// bulkDetachDeleteByIDs runs set-based DETACH DELETE in chunks (same Cypher shape as
// cascadeDelete). Does not use DETACH DELETE … RETURN (Memgraph-fragile); leaf deletes
// are idempotent — requested IDs are treated as success after a successful statement.
func (g *GraphObjectStorage) bulkDetachDeleteByIDs(ctx context.Context, ids []string, idIndex map[string]int, result *BulkResult) error {
	_ = idIndex
	for start := 0; start < len(ids); start += graphBulkSetDeleteBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := start + graphBulkSetDeleteBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		if err := g.detachDeleteEntitiesByIDs(ctx, chunk); err != nil {
			return errfmt.Newf(ConstStreamFailedToCommitBulkDeleteTransaction).Wrap(err)
		}
		for _, id := range chunk {
			result.SuccessCount++
			result.Results = append(result.Results, map[string]any{objects.FieldKeyID: id})
		}
	}
	return nil
}

func (g *GraphObjectStorage) detachDeleteEntitiesByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	q := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query: `
			MATCH (n:Entity)
			WHERE n.id IN $deleteIds
			DETACH DELETE n
		`,
		Params: map[string]any{
			"deleteIds": ids,
		},
	}
	_, err := g.conn.ExecuteQuery(ctx, q)
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.bulkDetachDelete ExecuteQuery", err).Log()
		}
		return errfmt.Newf("bulk detach delete failed").Wrap(err)
	}
	return nil
}

// ensureEntityIDIndex best-effort creates an index on Entity.id so IN $deleteIds
// set-deletes stay O(log n) locally. Errors are ignored (index may already exist).
func (g *GraphObjectStorage) ensureEntityIDIndex(ctx context.Context) error {
	q := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    "CREATE INDEX ON :Entity(id)",
		Params:   nil,
	}
	_, err := g.conn.ExecuteQuery(ctx, q)
	if err != nil && isGraphRetryable(err) {
		logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.bulkUnlink ExecuteQuery", err).Log()
	}
	return err
}

// bulkDeleteNonLeafLegacy deletes non-leaf IDs one-by-one with dependency checks.
// Kept for correctness; litter purge (scheduler_job) uses the set-based leaf path.
func (g *GraphObjectStorage) bulkDeleteNonLeafLegacy(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, idIndex map[string]int, result *BulkResult) error {
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		idx := idIndex[id]
		if err := g.Delete(ctx, secCtx, id, false); err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID: id, Index: idx, Error: err,
				Message: fmt.Sprintf(ConstStreamFailedToDeleteObjectStrVal, id, err),
			})
			continue
		}
		result.SuccessCount++
		result.Results = append(result.Results, map[string]any{objects.FieldKeyID: id})
	}
	return nil
}
