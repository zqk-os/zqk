package storage

import (
	"context"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// TRACK: REDACTED — iterative BFS cascade delete (no recursive Delete; cycle-safe).

func filterBlockingDependents(dependents []string) []string {
	if len(dependents) == 0 {
		return dependents
	}
	out := make([]string, 0, len(dependents))
	for _, depID := range dependents {
		if strings.HasPrefix(depID, PrefixAudit) || strings.HasPrefix(depID, "CHA-") {
			continue
		}
		out = append(out, depID)
	}
	return out
}

type cascadeEdge struct {
	id       string
	parentID string // object being deleted that this id references
}

// cascadeDeleteDependentsBFS expands dependents with an explicit queue (BFS), unlinks
// association/composition parents, then hard-deletes the cascade set leaf-first so cycles
// and deep trees cannot blow the stack or leave orphans from abandoned recursion.
func (f *FileObjectStorage) cascadeDeleteDependentsBFS(ctx context.Context, secCtx *pkgctx.SecurityContext, rootID string) error {
	seen := map[string]bool{rootID: true}
	cascadeSet := make(map[string]bool)

	queue := make([]cascadeEdge, 0, 8)
	seed, err := f.findDependents(ctx, rootID, "")
	if err != nil {
		return errfmt.Newf(ErrMsgCheckDeps).Wrap(err)
	}
	for _, depID := range filterBlockingDependents(seed) {
		queue = append(queue, cascadeEdge{id: depID, parentID: rootID})
	}

	for len(queue) > 0 {
		edge := queue[0]
		queue = queue[1:]
		if seen[edge.id] {
			continue
		}

		dependent, depErr := f.Read(ctx, secCtx, edge.id)
		if depErr != nil {
			continue
		}
		dependentKind, _ := dependent[objects.FieldKeyKind].(string)
		if dependentKind == emptyValue {
			continue
		}

		if !ShouldCascadeDeleteDependent(dependent, edge.parentID) {
			if err := UnlinkReferencesFromDependents(ctx, secCtx, f, edge.parentID, []string{edge.id}); err != nil {
				return errfmt.Errorf("failed to unlink parent reference in %s: %w", edge.id, err)
			}
			seen[edge.id] = true
			continue
		}

		seen[edge.id] = true
		cascadeSet[edge.id] = true

		childDeps, depErr := f.findDependents(ctx, edge.id, dependentKind)
		if depErr != nil {
			return errfmt.Newf(ErrMsgCheckDeps).Wrap(depErr)
		}
		for _, childID := range filterBlockingDependents(childDeps) {
			if !seen[childID] {
				queue = append(queue, cascadeEdge{id: childID, parentID: edge.id})
			}
		}
	}

	if len(cascadeSet) == 0 {
		return nil
	}

	// Leaf-first: repeatedly delete members with no remaining dependents still in cascadeSet.
	remaining := make(map[string]bool, len(cascadeSet))
	for id := range cascadeSet {
		remaining[id] = true
	}
	for len(remaining) > 0 {
		progress := false
		for id := range remaining {
			deps, depErr := f.findDependents(ctx, id, "")
			if depErr != nil {
				return errfmt.Newf(ErrMsgCheckDeps).Wrap(depErr)
			}
			blocked := false
			for _, d := range filterBlockingDependents(deps) {
				if remaining[d] {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}
			// cascade=false: closure already expanded; avoids re-entering BFS.
			if err := f.Delete(ctx, secCtx, id, false); err != nil {
				return errfmt.Errorf(ErrMsgCascadeDeleteFail, id, err)
			}
			delete(remaining, id)
			progress = true
		}
		if !progress {
			// Cycle within remaining: force-delete one node (dependents outside set already unlinked/gone).
			var forceID string
			for id := range remaining {
				forceID = id
				break
			}
			if forceID == emptyValue {
				break
			}
			// Strip refs from peers still in the set, then delete.
			peers := make([]string, 0, len(remaining))
			for id := range remaining {
				if id != forceID {
					peers = append(peers, id)
				}
			}
			if err := UnlinkReferencesFromDependents(ctx, secCtx, f, forceID, peers); err != nil {
				return errfmt.Errorf("failed to break cascade cycle at %s: %w", forceID, err)
			}
			if err := f.Delete(ctx, secCtx, forceID, false); err != nil {
				return errfmt.Errorf(ErrMsgCascadeDeleteFail, forceID, err)
			}
			delete(remaining, forceID)
		}
	}
	return nil
}
