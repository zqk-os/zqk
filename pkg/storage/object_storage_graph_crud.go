package storage

import (
	"context"
	"fmt"
	"slices"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func (g *GraphObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	// Extract kind
	kind, ok := obj[objects.FieldKeyKind].(string)
	if !ok || kind == emptyValue {
		return errfmt.Errorf(ErrMsgObjectNeedsKind)
	}

	// Check permission
	if err := g.checkPermission(secCtx, "write", kind); err != nil {
		return err
	}

	// Ensure object has valid ID
	id, err := g.ensureObjectID(ctx, obj, kind)
	if err != nil {
		return err
	}

	// Check if object already exists
	label := toLabel(kind)
	existingNode, err := g.conn.GetNode(ctx, id, []string{label, "Entity"})
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.Create GetNode: %v", err), nil).Log()
		}
		// If error is "not found", it's fine - we're creating it
		if gerr, ok := err.(*provider.GraphError); !ok || !gerr.IsNodeNotFound() {
			// Error means query failed - this is a real error, not "not found"
			return errfmt.Newf(ConstStreamFailedToCheckIfObjectExists).Wrap(err)
		}
	}
	if existingNode != nil {
		// Node exists, return error
		return ErrObjectExists
	}

	// Ensure metadata
	g.ensureObjectMetadata(obj, secCtx, true)

	// Validate object (spec, lifecycle, references)
	if err := g.validateObject(ctx, obj, kind, ""); err != nil {
		return errfmt.Newf(ConstStreamValidationFailed).Wrap(err)
	}

	// Convert to node and create
	node, err := g.objectToNode(obj)
	if err != nil {
		return err
	}

	err = g.conn.CreateNode(ctx, node)
	if err != nil && isGraphRetryable(err) {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.Create CreateNode: %v", err), nil).Log()
	}
	return err
}

// Read retrieves an object by ID from the graph
func (g *GraphObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	// Infer kind from ID
	if err := g.idValidator.LoadPatterns(); err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	kind := g.idValidator.InferKindFromID(id)
	if kind == emptyValue {
		return nil, errfmt.Errorf(ConstStreamCouldNotInferKindFromIdStr, id)
	}

	// Check permission
	if err := g.checkPermission(secCtx, "read", kind); err != nil {
		return nil, err
	}

	// Get node from graph
	label := toLabel(kind)
	node, err := g.conn.GetNode(ctx, id, []string{label, "Entity"})
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.Read GetNode: %v", err), nil).Log()
		}
		return nil, ErrObjectNotFound
	}
	if node == nil {
		return nil, ErrObjectNotFound
	}

	// Convert to object
	obj := g.nodeToObject(node)
	if obj == nil {
		return nil, ErrObjectNotFound
	}
	return obj, nil
}

// Update updates an existing object in the graph
func (g *GraphObjectStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	// Read existing object
	existing, err := g.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}

	kind, ok := existing[objects.FieldKeyKind].(string)
	if !ok {
		return errfmt.Errorf(ConstStreamObjectMissingKindField2)
	}

	// Check permission
	if err := g.checkPermission(secCtx, "write", kind); err != nil {
		return err
	}

	if s := objects.GetString(updates, objects.FieldKeyStatus); s != "" {
		if err := checkVerificationOutcomeAuthority(kind, secCtx, s); err != nil {
			return err
		}
	}

	// Optimistic locking: check updated_at if provided in updates
	if expectedUpdatedAt := objects.GetString(updates, ConstStreamExpectedUpdatedAt); expectedUpdatedAt != "" {
		actualUpdatedAt, _ := existing[objects.FieldKeyUpdatedAt].(string)
		if actualUpdatedAt != expectedUpdatedAt {
			return ErrVersionConflict
		}
		// Remove from updates (it's a control field, not part of the object)
		delete(updates, ConstStreamExpectedUpdatedAt)
	}

	// Check if this is a built-in object and user has admin role
	isBuiltIn := IsBuiltIn(existing)
	hasAdminRole := slices.Contains(secCtx.Roles, "admin")

	// Get old status before merge for lifecycle transition validation
	oldState, _ := existing[objects.FieldKeyStatus].(string)

	// Merge updates into existing object
	for k, v := range updates {
		// CLI --unset-field: remove key from persisted object
		if IsFieldUnset(v) {
			delete(existing, k)
			continue
		}
		// Don't allow updating immutable fields, unless:
		// - Object is built-in AND user has admin role (for internal command)
		// - Even then, id and kind remain immutable (too fundamental)
		if k == objects.FieldKeyID || k == objects.FieldKeyKind {
			// id and kind are always immutable
			continue
		}
		if k == objects.FieldKeyCreatedAt || k == objects.FieldKeyCreatedBy {
			// Allow updating created_at/created_by for built-in objects with admin role
			if isBuiltIn && hasAdminRole {
				existing[k] = v
			}
			// Otherwise, skip (immutable)
			continue
		}
		if mergeMapPatchIntoExisting(existing, k, v) {
			continue
		}
		existing[k] = v
	}

	// Ensure metadata
	g.ensureObjectMetadata(existing, secCtx, false)

	// Validate object (spec, lifecycle, references)
	if err := g.validateObject(ctx, existing, kind, oldState); err != nil {
		return errfmt.Newf(ConstStreamValidationFailed).Wrap(err)
	}

	// Update node in graph
	nodeUpdates := provider.NodeUpdates{
		Properties: make(map[string]any),
	}

	// Copy all properties except id and kind
	for k, v := range existing {
		if k != objects.FieldKeyID && k != objects.FieldKeyKind {
			nodeUpdates.Properties[k] = v
		}
	}

	err = g.conn.UpdateNode(ctx, id, nodeUpdates)
	if err != nil && isGraphRetryable(err) {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.Update UpdateNode: %v", err), nil).Log()
	}
	return err
}

// Move moves an object to a different directory/kind while preserving history
func (g *GraphObjectStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id, newKind string, updateReferences bool) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	// Require CLI authorization for moves
	if !IsCLIOperation(ctx, secCtx) {
		return errfmt.Errorf(ConstStreamMoveOperationsMustBePerformedThroughCli)
	}

	// Read existing object
	existing, err := g.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}

	oldKind, ok := existing[objects.FieldKeyKind].(string)
	if !ok {
		return errfmt.Errorf(ConstStreamObjectMissingKindField2)
	}

	// Validate new kind
	if newKind == emptyValue {
		return errfmt.Errorf(ConstStreamNewKindCannotBeEmpty)
	}

	if oldKind == newKind {
		return errfmt.Errorf(ConstStreamObjectIsAlreadyOfKindStrNoMoveNeeded, newKind)
	}

	// Validate new kind exists
	newDir := objects.GetDirectoryFromKind(newKind)
	if newDir == emptyValue {
		return errfmt.Errorf(ConstStreamInvalidKindStrNoDirectoryMappingFound, newKind)
	}

	// Check permissions
	if err := g.checkPermission(secCtx, "write", oldKind); err != nil {
		return errfmt.Errorf(ConstStreamPermissionDeniedForSourceKindStrErr, oldKind, err)
	}
	if err := g.checkPermission(secCtx, "write", newKind); err != nil {
		return errfmt.Errorf(ConstStreamPermissionDeniedForTargetKindStrErr, newKind, err)
	}

	// Update object kind
	existing[objects.FieldKeyKind] = newKind

	// Update metadata (updated_at, updated_by)
	now := zqktime.NowRFC3339UTC()
	existing[objects.FieldKeyUpdatedAt] = now
	if secCtx != nil && secCtx.AccountID != emptyValue {
		existing[objects.FieldKeyUpdatedBy] = secCtx.AccountID
	}

	// Validate object with new kind
	status, _ := existing[objects.FieldKeyStatus].(string)
	if err := g.validateObject(ctx, existing, newKind, status); err != nil {
		return errfmt.Newf(ConstStreamValidationFailedForNewKind).Wrap(err)
	}

	// Get old and new labels
	oldLabel := toLabel(oldKind)
	newLabel := toLabel(newKind)

	// Update node in graph (change kind property and labels)
	updates := provider.NodeUpdates{
		Properties: map[string]any{
			objects.FieldKeyKind:      newKind,
			objects.FieldKeyUpdatedAt: now,
		},
		RemoveLabels: []string{oldLabel}, // Remove old label
		AddLabels:    []string{newLabel}, // Add new label
	}

	if err := g.conn.UpdateNode(ctx, id, updates); err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.Move UpdateNode: %v", err), nil).Log()
		}
		return errfmt.Newf(ConstStreamFailedToUpdateNodeInGraph).Wrap(err)
	}

	// Update references if requested
	if updateReferences {
		// Find all nodes that reference this node and update their references
		// This would require querying for incoming relationships
		// For now, this is a placeholder - full implementation would:
		// 1. Query for all nodes with reference properties pointing to this node
		// 2. Update those reference properties to use the new kind
		_ = updateReferences // Suppress unused variable warning
	}

	return nil
}

// Rename changes an object's ID (same kind). ITEM-851.
// Not implemented for graph storage; use file storage for rename operations.
func (g *GraphObjectStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	_ = updateReferences
	return errfmt.Errorf(ConstStreamRenameNotImplementedForGraphStorageUseFileBased)
}

// Delete deletes an object from the graph
func (g *GraphObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	// Require CLI authorization for deletions
	// This prevents direct API calls from deleting objects without going through CLI
	if !IsCLIOperation(ctx, secCtx) {
		return errfmt.Errorf(ConstStreamDeleteOperationsMustBePerformedThroughCli)
	}

	// Read existing object to get kind
	existing, err := g.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}

	kind, ok := existing[objects.FieldKeyKind].(string)
	if !ok {
		return errfmt.Errorf(ConstStreamObjectMissingKindField2)
	}

	// Check permission (requires explicit delete permission)
	if err := g.checkPermission(secCtx, OpDelete, kind); err != nil {
		return errfmt.Errorf(ConstStreamPermissionDeniedErrDeleteOperationsRequire, err)
	}

	var dependents []string
	if cascade {
		if err := g.cascadeDelete(ctx, secCtx, id, kind); err != nil {
			return errfmt.Newf(ConstStreamCascadeDeleteFailed).Wrap(err)
		}
		return nil
	}

	var depErr error
	dependents, depErr = g.findDependents(ctx, id, kind)
	if depErr != nil {
		return errfmt.Newf(ConstStreamFailedToCheckDependencies).Wrap(depErr)
	}
	if len(dependents) > 0 && UnlinkReferencesBeforeDelete(ctx) {
		if err := UnlinkReferencesFromDependents(ctx, secCtx, g, id, dependents); err != nil {
			return errfmt.Newf(ConstStreamUnlinkReferencesBeforeDeleteFailed).Wrap(err)
		}
		dependents, depErr = g.findDependents(ctx, id, kind)
		if depErr != nil {
			return errfmt.Newf(ConstStreamFailedToCheckDependenciesAfterUnlink).Wrap(depErr)
		}
	}
	if len(dependents) > 0 {
		return errfmt.Errorf(ConstStreamCannotDeleteObjectStrIntDependentObjectSStill, id, len(dependents))
	}

	// Create audit event BEFORE deletion (so we have the object data)
	// Best effort - don't fail deletion if audit event creation fails
	// For graph storage, try to get project root from context
	projectRoot := ""
	if projectRootVal := ctx.Value("project_root"); projectRootVal != nil {
		if pr, ok := projectRootVal.(string); ok {
			projectRoot = pr
		}
	}
	var _err_83570882 = createDeleteAuditEvent(ctx, projectRoot, id, kind, "", cascade, secCtx, dependents, nil)
	if _err_83570882 !=

		// Delete node from graph
		nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83570882).Log()
	}

	label := toLabel(kind)
	err = g.conn.DeleteNode(ctx, id, []string{label, "Entity"})
	if err != nil && isGraphRetryable(err) {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.Delete DeleteNode: %v", err), nil).Log()
	}
	return err
}

// findDependents finds all objects that reference the given object
func (g *GraphObjectStorage) findDependents(ctx context.Context, id, _ string) ([]string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	// Query graph for nodes that have edges or properties referencing this ID
	// This is a simplified implementation - for better performance, we could use an index
	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query: `
			MATCH (n:Entity)
			WHERE n.id <> $targetId
			AND (
				EXISTS {
					MATCH (n)-[r]->(target {id: $targetId})
				}
				OR ANY(key IN keys(n) WHERE 
					n[key] = $targetId
					OR (toUpper(valueType(n[key])) = "LIST" AND $targetId IN n[key])
				)
			)
			RETURN n.id AS id
		`,
		Params: map[string]any{
			"targetId": id,
		},
	}

	result, err := g.conn.ExecuteQuery(ctx, query)
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.findDependents ExecuteQuery: %v", err), nil).Log()
		}
		return nil, errfmt.Newf(ConstStreamFailedToQueryDependents).Wrap(err)
	}

	var dependents []string
	// QueryResult has Nodes, Edges, and Rows
	// For this query, we expect rows with "id" field
	for _, row := range result.Rows {
		if idValue := objects.GetString(row, objects.FieldKeyID); idValue != "" {
			dependents = append(dependents, idValue)
		}
	}

	return dependents, nil
}

// cascadeDelete deletes an object and all objects that reference it iteratively using a single Cypher query
func (g *GraphObjectStorage) cascadeDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, startID, startKind string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query: `
			MATCH (n:Entity {id: $startId})<-[*0..]-(dep:Entity)
			RETURN dep.id AS id, dep.kind AS kind
		`,
		Params: map[string]any{
			"startId": startID,
		},
	}

	result, err := g.conn.ExecuteQuery(ctx, query)
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.cascadeDelete ExecuteQuery: %v", err), nil).Log()
		}
		return errfmt.Newf(ConstStreamCascadeDeleteFailed).Wrap(err)
	}

	var toDeleteIDs []string
	var toDeleteKinds []string
	seen := make(map[string]bool)

	for _, row := range result.Rows {
		idVal := objects.GetString(row, "id")
		kindVal := objects.GetString(row, "kind")
		if idVal != "" && !seen[idVal] {
			seen[idVal] = true
			toDeleteIDs = append(toDeleteIDs, idVal)
			toDeleteKinds = append(toDeleteKinds, kindVal)
		}
	}

	// Create audit event for the main object being deleted (cascade)
	projectRoot := ""
	if projectRootVal := ctx.Value("project_root"); projectRootVal != nil {
		if pr, ok := projectRootVal.(string); ok {
			projectRoot = pr
		}
	}

	var directDeps []string
	if len(toDeleteIDs) > 1 {
		directDeps, _ = g.findDependents(ctx, startID, startKind)
	}

	var _err_83574564 = createDeleteAuditEvent(ctx, projectRoot, startID, startKind, "", true, secCtx, directDeps, nil)
	if _err_83574564 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83574564).Log()
	}

	if len(toDeleteIDs) > 0 {
		deleteQuery := provider.Query{
			Language: provider.QueryLanguageCypher,
			Query: `
				MATCH (n:Entity)
				WHERE n.id IN $deleteIds
				DETACH DELETE n
			`,
			Params: map[string]any{
				"deleteIds": toDeleteIDs,
			},
		}

		_, err = g.conn.ExecuteQuery(ctx, deleteQuery)
		if err != nil {
			if isGraphRetryable(err) {
				logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.cascadeDelete DeleteQuery: %v", err), nil).Log()
			}
			return errfmt.Newf("bulk detach delete failed").Wrap(err)
		}
	}

	return nil
}

func isGraphRetryable(err error) bool {
	if gerr, ok := err.(*provider.GraphError); ok {
		return gerr.IsRetryable()
	}
	return false
}
