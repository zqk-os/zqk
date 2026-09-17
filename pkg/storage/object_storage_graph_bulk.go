package storage

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// graphBulkUNWINDBatchSize is the chunk size for BulkCreate/BulkUpdate via
// conn.ExecuteBatch (Memgraph UNWIND $batch). Matches the create_node chunker
// in pkg/graph/memgraph/connection.go (≤500).
const graphBulkUNWINDBatchSize = 500

// graphBulkSetDeleteBatchSize is the chunk size for set-based DETACH DELETE
// (WHERE n.id IN $deleteIds). One Cypher statement deletes the whole chunk —
// world-class local throughput; do not use N×DeleteNode for bulk.
// See docs/architecture/MEMGRAPH_BULK_OPS_GUIDE.md (DETACH DELETE = 500–1000).
const graphBulkSetDeleteBatchSize = 1000

type preparedBulkCreate struct {
	index int
	obj   map[string]any
	node  provider.Node
}

// BulkCreate creates multiple objects via Memgraph ExecuteBatch (UNWIND chunks),
// not N×CreateNode inside 50-sized txs. Prep/validate stays per object; the
// write path is O(chunks). See docs/architecture/MEMGRAPH_BULK_OPS_GUIDE.md §3.A.
func (g *GraphObjectStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objList []map[string]any) (*BulkResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
	}

	result := &BulkResult{
		TotalCount: len(objList),
		Results:    make([]map[string]any, 0, len(objList)),
		Errors:     make([]BulkOperationError, 0),
	}
	if len(objList) == 0 {
		return result, nil
	}

	ready := make([]preparedBulkCreate, 0, len(objList))
	for i, obj := range objList {
		node, prepErr := g.prepareBulkCreateNode(ctx, secCtx, obj)
		if prepErr != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				Index:   i,
				Error:   prepErr,
				Message: fmt.Sprintf(ConstStreamFailedToCreateObjectAtIndexIntVal, i, prepErr),
			})
			continue
		}
		ready = append(ready, preparedBulkCreate{index: i, obj: obj, node: node})
	}
	if len(ready) == 0 {
		return result, nil
	}

	for start := 0; start < len(ready); start += graphBulkUNWINDBatchSize {
		end := start + graphBulkUNWINDBatchSize
		if end > len(ready) {
			end = len(ready)
		}
		chunk := ready[start:end]
		ops := make([]provider.Operation, 0, len(chunk))
		for _, p := range chunk {
			ops = append(ops, provider.Operation{Type: "create_node", Data: p.node})
		}
		batchRes, batchErr := g.conn.ExecuteBatch(ctx, ops)
		if batchErr != nil {
			if isGraphRetryable(batchErr) {
				logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.BulkCreate ExecuteBatch", batchErr).Log()
			}
			for _, p := range chunk {
				result.FailureCount++
				result.Errors = append(result.Errors, BulkOperationError{
					Index:   p.index,
					ID:      p.node.ID,
					Error:   batchErr,
					Message: fmt.Sprintf(ConstStreamFailedToCreateObjectAtIndexIntVal, p.index, batchErr),
				})
			}
			continue
		}
		// ExecuteBatch UNWIND treats the chunk as one unit on success; map 1:1 to prep rows.
		successN := 0
		if batchRes != nil {
			successN = batchRes.SuccessCount
		}
		if successN <= 0 && batchRes != nil && batchRes.FailureCount == 0 {
			// Mocks / drivers that omit counts but return nil error: treat full chunk OK.
			successN = len(chunk)
		}
		if successN > len(chunk) {
			successN = len(chunk)
		}
		for j, p := range chunk {
			if j < successN {
				result.SuccessCount++
				kind, _ := p.obj[objects.FieldKeyKind].(string)
				result.Results = append(result.Results, map[string]any{
					objects.FieldKeyID:   p.node.ID,
					objects.FieldKeyKind: kind,
				})
				continue
			}
			errMsg := errfmt.Errorf("bulk create batch incomplete at index %d", p.index)
			if batchRes != nil && len(batchRes.Errors) > 0 {
				errMsg = batchRes.Errors[0]
			}
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				Index:   p.index,
				ID:      p.node.ID,
				Error:   errMsg,
				Message: fmt.Sprintf(ConstStreamFailedToCreateObjectAtIndexIntVal, p.index, errMsg),
			})
		}
	}
	return result, nil
}

// prepareBulkCreateNode runs Create's local prep (ID, exists, validate, objectToNode)
// without writing. Used by BulkCreate before ExecuteBatch UNWIND.
func (g *GraphObjectStorage) prepareBulkCreateNode(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) (provider.Node, error) {
	kind, ok := obj[objects.FieldKeyKind].(string)
	if !ok || kind == emptyValue {
		return provider.Node{}, errfmt.Errorf(ErrMsgObjectNeedsKind)
	}
	if err := g.checkPermission(secCtx, "write", kind); err != nil {
		return provider.Node{}, err
	}
	if _, err := g.ensureObjectID(ctx, obj, kind); err != nil {
		return provider.Node{}, err
	}
	label := toLabel(kind)
	id, _ := obj[objects.FieldKeyID].(string)
	existingNode, err := g.conn.GetNode(ctx, id, []string{label, "Entity"})
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.BulkCreate GetNode", err).Log()
		}
		gerr := &provider.GraphError{}
		if errors.As(err, &gerr) {
			if gerr.Code != provider.ErrorCodeNodeNotFound {
				return provider.Node{}, errfmt.Newf(ConstStreamFailedToCheckIfObjectExists).Wrap(err)
			}
		} else {
			return provider.Node{}, errfmt.Newf(ConstStreamFailedToCheckIfObjectExists).Wrap(err)
		}
	}
	if existingNode != nil {
		return provider.Node{}, ErrObjectExists
	}
	g.ensureObjectMetadata(ctx, obj, secCtx, true)
	if err := g.validateObject(ctx, obj, kind, ""); err != nil {
		return provider.Node{}, errfmt.Newf(ConstStreamValidationFailed).Wrap(err)
	}
	return g.objectToNode(obj)
}

// BulkUpdate updates multiple objects via Memgraph ExecuteBatch (UNWIND chunks),
// not N×UpdateNode inside 50-sized txs. Prep/validate stays per object; the write
// path is O(chunks).
func (g *GraphObjectStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
	}

	result := &BulkResult{
		TotalCount: len(updates),
		Results:    make([]map[string]any, 0, len(updates)),
		Errors:     make([]BulkOperationError, 0),
	}
	if len(updates) == 0 {
		return result, nil
	}

	type preparedUpdate struct {
		index int
		id    string
		data  map[string]any // ExecuteBatch update_node payload
	}
	ready := make([]preparedUpdate, 0, len(updates))
	for i, item := range updates {
		payload, prepErr := g.prepareBulkUpdatePayload(ctx, secCtx, item.ID, item.Updates)
		if prepErr != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      item.ID,
				Index:   i,
				Error:   prepErr,
				Message: fmt.Sprintf(ConstStreamFailedToUpdateObjectStrVal, item.ID, prepErr),
			})
			continue
		}
		ready = append(ready, preparedUpdate{index: i, id: item.ID, data: payload})
	}
	if len(ready) == 0 {
		return result, nil
	}

	for start := 0; start < len(ready); start += graphBulkUNWINDBatchSize {
		end := start + graphBulkUNWINDBatchSize
		if end > len(ready) {
			end = len(ready)
		}
		chunk := ready[start:end]
		ops := make([]provider.Operation, 0, len(chunk))
		for _, p := range chunk {
			ops = append(ops, provider.Operation{Type: "update_node", Data: p.data})
		}
		batchRes, batchErr := g.conn.ExecuteBatch(ctx, ops)
		if batchErr != nil {
			if isGraphRetryable(batchErr) {
				logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.BulkUpdate ExecuteBatch", batchErr).Log()
			}
			for _, p := range chunk {
				result.FailureCount++
				result.Errors = append(result.Errors, BulkOperationError{
					ID: p.id, Index: p.index, Error: batchErr,
					Message: fmt.Sprintf(ConstStreamFailedToUpdateObjectStrVal, p.id, batchErr),
				})
			}
			continue
		}
		successN := 0
		if batchRes != nil {
			successN = batchRes.SuccessCount
		}
		if successN <= 0 && batchRes != nil && batchRes.FailureCount == 0 {
			successN = len(chunk)
		}
		if successN > len(chunk) {
			successN = len(chunk)
		}
		for j, p := range chunk {
			if j < successN {
				result.SuccessCount++
				result.Results = append(result.Results, map[string]any{objects.FieldKeyID: p.id})
				continue
			}
			errMsg := errfmt.Errorf("bulk update batch incomplete for %s", p.id)
			if batchRes != nil && len(batchRes.Errors) > 0 {
				errMsg = batchRes.Errors[0]
			}
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID: p.id, Index: p.index, Error: errMsg,
				Message: fmt.Sprintf(ConstStreamFailedToUpdateObjectStrVal, p.id, errMsg),
			})
		}
	}
	return result, nil
}

// prepareBulkUpdatePayload runs Update's local prep (read/merge/validate) and
// returns an ExecuteBatch update_node payload without writing.
func (g *GraphObjectStorage) prepareBulkUpdatePayload(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) (map[string]any, error) {
	existing, err := g.Read(ctx, secCtx, id)
	if err != nil {
		return nil, err
	}
	kind, ok := existing[objects.FieldKeyKind].(string)
	if !ok {
		return nil, errfmt.Errorf(ConstStreamObjectMissingKindField2)
	}
	if err := g.checkPermission(secCtx, "write", kind); err != nil {
		return nil, err
	}
	// Copy so we do not mutate caller maps.
	patched := make(map[string]any, len(updates))
	for k, v := range updates {
		patched[k] = v
	}
	if s := objects.GetString(patched, objects.FieldKeyStatus); s != "" {
		if err := checkVerificationOutcomeAuthority(kind, secCtx, s); err != nil {
			return nil, err
		}
	}
	if expectedUpdatedAt := objects.GetString(patched, ConstStreamExpectedUpdatedAt); expectedUpdatedAt != "" {
		actualUpdatedAt, _ := existing[objects.FieldKeyUpdatedAt].(string)
		if actualUpdatedAt != expectedUpdatedAt {
			return nil, ErrVersionConflict
		}
		delete(patched, ConstStreamExpectedUpdatedAt)
	}
	isBuiltIn := IsBuiltIn(existing)
	hasAdminRole := slices.Contains(secCtx.Roles, "admin")
	oldState, _ := existing[objects.FieldKeyStatus].(string)
	for k, v := range patched {
		if IsFieldUnset(v) {
			delete(existing, k)
			continue
		}
		if k == objects.FieldKeyID || k == objects.FieldKeyKind {
			continue
		}
		if k == objects.FieldKeyCreatedAt || k == objects.FieldKeyCreatedBy {
			if isBuiltIn && hasAdminRole {
				existing[k] = v
			}
			continue
		}
		if mergeMapPatchIntoExisting(existing, k, v) {
			continue
		}
		existing[k] = v
	}
	g.ensureObjectMetadata(ctx, existing, secCtx, false)
	if err := g.validateObject(ctx, existing, kind, oldState); err != nil {
		return nil, errfmt.Newf(ConstStreamValidationFailed).Wrap(err)
	}
	props := make(map[string]any, len(existing))
	for k, v := range existing {
		if k != objects.FieldKeyID && k != objects.FieldKeyKind {
			props[k] = v
		}
	}
	return map[string]any{
		objects.FieldKeyID: id,
		"properties":       props,
	}, nil
}

// BulkGet retrieves multiple objects by ID
func (g *GraphObjectStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()

	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	for i, id := range ids {
		obj, err := g.Read(ctx, secCtx, id)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      id,
				Index:   i,
				Error:   err,
				Message: fmt.Sprintf(ConstStreamFailedToReadObjectStrVal, id, err),
			})
			continue
		}

		result.SuccessCount++
		result.Results = append(result.Results, obj)
	}

	return result, nil
}

// BulkDelete deletes multiple objects via set-based Cypher (WHERE id IN … DETACH DELETE)
// in graphBulkSetDeleteBatchSize chunks. This is the world-class local path: O(chunks)
// round-trips, not O(N) Read+DeleteNode. Leaf kinds skip per-ID dependency scans.
// Cascade still expands per root (rarer than litter purge).
