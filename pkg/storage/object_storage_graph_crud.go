package storage

import (
	"context"
	"errors"
	"slices"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/kernelcas"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func (g *GraphObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	// Kernel Mutation Pipeline entry (COMMIT re-enters with kernelcas.WithCommit).
	// TRACK: BLI-1785784864671436000-071adcbe — Graph was a membrane bypass.
	if !kernelcas.IsCommit(ctx) {
		kind, _ := obj[objects.FieldKeyKind].(string)
		id, _ := obj[objects.FieldKeyID].(string)
		return kernelcas.RunCreate(ctx, nil, &kernelcas.Mutation{
			Kind:   kind,
			ID:     id,
			Intent: kernelcas.IntentCreate,
			Reason: pkgctx.GetLifecycleBreakGlassReason(ctx),
			CommitFn: func(c context.Context) error {
				return g.Create(c, secCtx, obj)
			},
		})
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
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.Create GetNode", err).Log()
		}
		// If error is "not found", it's fine - we're creating it
		gerr := &provider.GraphError{}
		if errors.As(err, &gerr) {
			if gerr.Code != provider.ErrorCodeNodeNotFound {
				return errfmt.Newf(ConstStreamFailedToCheckIfObjectExists).Wrap(err)
			}
		} else {
			return errfmt.Newf(ConstStreamFailedToCheckIfObjectExists).Wrap(err)
		}
	}
	if existingNode != nil {
		// Node exists, return error
		return ErrObjectExists
	}

	// Ensure metadata
	g.ensureObjectMetadata(ctx, obj, secCtx, true)

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
		logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.Create CreateNode", err).Log()
	}
	return err
}

// Read retrieves an object by ID from the graph
func (g *GraphObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()

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
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.Read GetNode", err).Log()
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
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	if !kernelcas.IsCommit(ctx) {
		kind := ""
		if existing, err := g.Read(ctx, secCtx, id); err == nil && existing != nil {
			kind, _ = existing[objects.FieldKeyKind].(string)
		}
		intent := kernelcas.IntentUpdateFields
		if _, hasStatus := updates[objects.FieldKeyStatus]; hasStatus {
			intent = kernelcas.IntentTransition
		}
		run := kernelcas.RunUpdate
		if intent == kernelcas.IntentTransition {
			run = kernelcas.RunTransition
		}
		return run(ctx, nil, &kernelcas.Mutation{
			Kind:   kind,
			ID:     id,
			Intent: intent,
			Reason: pkgctx.GetLifecycleBreakGlassReason(ctx),
			CommitFn: func(c context.Context) error {
				return g.Update(c, secCtx, id, updates)
			},
		})
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
	removeProperties := UnsetFieldKeys(updates)

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
	g.ensureObjectMetadata(ctx, existing, secCtx, false)

	// Validate object (spec, lifecycle, references)
	if err := g.validateObject(ctx, existing, kind, oldState); err != nil {
		return errfmt.Newf(ConstStreamValidationFailed).Wrap(err)
	}

	// Same property prep as Create (compression) so Update cannot drift graph
	// property keys from objectToNode / file SSOT projection.
	// TRACK: BLI-1785825613639964000-ba487700 — GFS P0c Update via objectToNode.
	node, err := g.objectToNode(existing)
	if err != nil {
		return err
	}
	nodeUpdates := provider.NodeUpdates{
		Properties:       node.Properties,
		RemoveProperties: removeProperties,
	}

	err = g.conn.UpdateNode(ctx, id, nodeUpdates)
	if err != nil && isGraphRetryable(err) {
		logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.Update UpdateNode", err).Log()
	}
	if err == nil {
		newState, _ := existing[objects.FieldKeyStatus].(string)
		if newState != emptyValue && oldState != newState {
			hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, g.projectRoot)
			if hookErr := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); hookErr != nil && !IsExpectedMissingErr(hookErr) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, hookErr).Log()
			}
		}
	}
	return err
}

// Move moves an object to a different directory/kind while preserving history
func (g *GraphObjectStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id, newKind string, updateReferences bool) error {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()

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
	if secCtx != nil {
		existing[objects.FieldKeyUpdatedBy] = pkgctx.ActorIDForAttribution(secCtx.AccountID)
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
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.Move UpdateNode", err).Log()
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

// Rename changes an object's ID (same kind). BLI-851.
// Not implemented for graph storage; use file storage for rename operations.
func (g *GraphObjectStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	_ = updateReferences
	return errfmt.Errorf(ConstStreamRenameNotImplementedForGraphStorageUseFileBased)
}

// Delete deletes an object from the graph
