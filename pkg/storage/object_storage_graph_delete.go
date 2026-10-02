// Extracted from object_storage_graph_crud.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"errors"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func (g *GraphObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	if kernelcas.IsCommit(ctx) {
		return g.deleteImpl(ctx, secCtx, id, cascade)
	}
	if !IsCLIOperation(ctx, secCtx) {
		return errfmt.Errorf(ConstStreamDeleteOperationsMustBePerformedThroughCli)
	}
	_, kind, err := readExistingKind(g, ctx, secCtx, id)
	if err != nil {
		return err
	}
	return kernelcas.RunErase(ctx, nil, &kernelcas.Mutation{
		Kind:       kind,
		ID:         id,
		Intent:     kernelcas.IntentEraseLogical,
		Cascade:    cascade,
		UnlinkRefs: UnlinkReferencesBeforeDelete(ctx),
		CommitFn: func(c context.Context) error {
			return g.deleteImpl(c, secCtx, id, cascade)
		},
	})
}

func (g *GraphObjectStorage) deleteImpl(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()

	// Require CLI authorization for deletions
	// This prevents direct API calls from deleting objects without going through CLI
	if !IsCLIOperation(ctx, secCtx) {
		return errfmt.Errorf(ConstStreamDeleteOperationsMustBePerformedThroughCli)
	}

	// Read existing object to get kind
	_, kind, err := readExistingKind(g, ctx, secCtx, id)
	if err != nil {
		return err
	}

	// Check permission (requires explicit delete permission)
	if err := g.checkPermission(secCtx, OpDelete, kind); err != nil {
		return errfmt.Errorf(ConstStreamPermissionDeniedErrDeleteOperationsRequire, err)
	}
	if err := denyCoreKernelHardDelete(ctx, secCtx, kind, id); err != nil {
		return err
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
		logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.Delete DeleteNode", err).Log()
	}
	return err
}

// findDependents finds all objects that reference the given object
func (g *GraphObjectStorage) findDependents(ctx context.Context, id, _ string) ([]string, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()

	// Query graph for nodes that have edges referencing this ID
	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query: `
			MATCH (n:Entity)-[]->(target:Entity {id: $targetId})
			RETURN n.id AS id
		`,
		Params: map[string]any{
			"targetId": id,
		},
	}

	result, err := g.conn.ExecuteQuery(ctx, query)
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.findDependents ExecuteQuery", err).Log()
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
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()

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
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.cascadeDelete ExecuteQuery", err).Log()
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
				logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.cascadeDelete DeleteQuery", err).Log()
			}
			return errfmt.Newf("bulk detach delete failed").Wrap(err)
		}
	}

	return nil
}

func isGraphRetryable(err error) bool {
	gerr := &provider.GraphError{}
	if errors.As(err, &gerr) {
		return gerr.IsRetryable()
	}
	return false
}
