package object

import (
	"context"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/storage"
)

// isCascadeEnabled returns true if --cascade or --family is set.
func isCascadeEnabled(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Flags() == nil {
		return false
	}
	cascade, _ := cmd.Flags().GetBool("cascade")
	family, _ := cmd.Flags().GetBool("family")
	return cascade || family
}

// extractObjectReferenceIDs extracts all referenced object IDs from reference fields.
func extractObjectReferenceIDs(obj map[string]any) []string {
	if obj == nil {
		return nil
	}
	var refs []string
	seen := make(map[string]bool)
	for k, v := range obj {
		if !strings.HasSuffix(k, "_ref") && !strings.HasSuffix(k, "_refs") && k != "dependencies" {
			continue
		}
		if k == objects.FieldKeyCreatedBy || k == objects.FieldKeyUpdatedBy || k == objects.FieldKeyAccountID || k == objects.FieldKeyCommitHashes || objects.IsLeftoverNonKernelRefField(k) {
			continue
		}
		for _, refID := range storage.ExtractReferenceIDs(v) {
			refID = strings.TrimSpace(refID)
			if refID != "" && !seen[refID] {
				seen[refID] = true
				refs = append(refs, refID)
			}
		}
	}
	return refs
}

// isDraftPlaneObject checks whether an object ID belongs to the draft plane.
func isDraftPlaneObject(ctx context.Context, secCtx *pkgctx.SecurityContext, proc *cli.Processor, id string) bool {
	if id == "" || proc == nil {
		return false
	}
	root := proc.ProjectRoot()
	if storage.IsDraftPlaneOnly(root, "", id) {
		return true
	}
	obj, err := proc.Storage().Read(ctx, secCtx, id)
	if err == nil && obj != nil {
		kind := koi.Kind(obj)
		status := koi.Status(obj)
		checker := objects.GetGlobalStatusChecker()
		if checker != nil && status != "" && checker.IsPreliminary(kind, status) {
			return true
		}
		if storage.IsDraftPlaneOnly(root, kind, id) || storage.ObjectDraftPlaneExists(root, kind, id) {
			return true
		}
	}
	return false
}

// expandCascadeDraftDependencies recursively discovers child draft objects referenced
// by seedIDs (e.g. criteria_refs, requirement_refs, test_case_refs) so that --cascade
// or --family can promote the whole connected draft family.
func expandCascadeDraftDependencies(ctx context.Context, secCtx *pkgctx.SecurityContext, proc *cli.Processor, seedIDs []string) ([]string, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	seen := make(map[string]bool, len(seedIDs)*2)
	var queue []string
	var result []string

	for _, rawID := range seedIDs {
		id, err := proc.ResolveSemanticArgument(ctx, "", rawID)
		if err != nil || id == "" {
			id = rawID
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
			queue = append(queue, id)
		}
	}

	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]

		obj, err := proc.Storage().Read(ctx, secCtx, currID)
		if err != nil || obj == nil {
			continue
		}

		refIDs := extractObjectReferenceIDs(obj)
		for _, refID := range refIDs {
			refIDResolved, err := proc.ResolveSemanticArgument(ctx, "", refID)
			if err == nil && refIDResolved != "" {
				refID = refIDResolved
			}
			if seen[refID] {
				continue
			}
			if isDraftPlaneObject(ctx, secCtx, proc, refID) {
				seen[refID] = true
				result = append(result, refID)
				queue = append(queue, refID)
			}
		}
	}

	return result, nil
}

// kindPromotionRank orders objects ontologically so that core specifications
// (Criteria, TestCases, Requirements) precede execution contracts (Epics, PriorityPlans, BacklogItems).
func kindPromotionRank(kind string) int {
	switch kind {
	case objects.KindCriteria:
		return 10
	case objects.KindTestCase:
		return 15
	case objects.KindRequirement:
		return 20
	case objects.KindPolicy:
		return 25
	case objects.KindGoal:
		return 30
	case objects.KindMilestone:
		return 35
	case objects.KindWorkstream:
		return 38
	case objects.KindEpic:
		return 40
	case objects.KindPriorityPlan:
		return 50
	case objects.KindBacklogItem:
		return 60
	case objects.KindAgentTask:
		return 70
	case objects.KindTechnicalDebt:
		return 80
	default:
		return 100
	}
}

// sortObjectIDsForMembranePromotion sorts IDs topologically based on object reference fields.
// If Object A references Object B in draft plane, Object B must be promoted before Object A
// across the membrane to avoid "cross-plane reference prohibited" validation errors.
// Cycles are handled gracefully using ontological kind ranking and deterministic tie-breaking.
func sortObjectIDsForMembranePromotion(ctx context.Context, secCtx *pkgctx.SecurityContext, proc *cli.Processor, rawIDs []string) ([]string, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(rawIDs) <= 1 {
		return rawIDs, nil
	}

	// Resolve semantic arguments and deduplicate while keeping input order
	ids := make([]string, 0, len(rawIDs))
	seen := make(map[string]bool, len(rawIDs))
	origIndex := make(map[string]int, len(rawIDs))
	for _, rawID := range rawIDs {
		resolved, err := proc.ResolveSemanticArgument(ctx, "", rawID)
		if err != nil || resolved == "" {
			resolved = rawID
		}
		if !seen[resolved] {
			seen[resolved] = true
			origIndex[resolved] = len(ids)
			ids = append(ids, resolved)
		}
	}

	if len(ids) <= 1 {
		return ids, nil
	}

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	// Read objects and detect references
	objectsMap := make(map[string]map[string]any, len(ids))
	kindsMap := make(map[string]string, len(ids))
	for _, id := range ids {
		obj, err := proc.Storage().Read(ctx, secCtx, id)
		if err == nil && obj != nil {
			objectsMap[id] = obj
			kindsMap[id] = koi.Kind(obj)
		}
	}

	// Graph representation:
	// If A references B, B must be promoted before A.
	// B is a dependency of A.
	// inDegree[A] counts how many unpromoted dependencies A is waiting for.
	// dependents[B] lists objects waiting on B.
	inDegree := make(map[string]int, len(ids))
	dependents := make(map[string][]string, len(ids))
	dependencies := make(map[string]map[string]bool, len(ids))

	for _, id := range ids {
		inDegree[id] = 0
		dependencies[id] = make(map[string]bool)
	}

	for _, id := range ids {
		obj := objectsMap[id]
		if obj == nil {
			continue
		}
		for _, refID := range extractObjectReferenceIDs(obj) {
			resolvedRef, err := proc.ResolveSemanticArgument(ctx, "", refID)
			if err == nil && resolvedRef != "" {
				refID = resolvedRef
			}
			if refID == id || !idSet[refID] {
				continue
			}
			if !dependencies[id][refID] {
				dependencies[id][refID] = true
				inDegree[id]++
				dependents[refID] = append(dependents[refID], id)
			}
		}
	}

	sortQueue := func(q []string) {
		sort.SliceStable(q, func(i, j int) bool {
			idA, idB := q[i], q[j]
			rankA := kindPromotionRank(kindsMap[idA])
			rankB := kindPromotionRank(kindsMap[idB])
			if rankA != rankB {
				return rankA < rankB
			}
			return origIndex[idA] < origIndex[idB]
		})
	}

	queue := make([]string, 0, len(ids))
	for _, id := range ids {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	sortQueue(queue)

	sorted := make([]string, 0, len(ids))
	visited := make(map[string]bool, len(ids))

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if visited[curr] {
			continue
		}
		visited[curr] = true
		sorted = append(sorted, curr)

		for _, dep := range dependents[curr] {
			inDegree[dep]--
			if inDegree[dep] == 0 && !visited[dep] {
				queue = append(queue, dep)
			}
		}
		sortQueue(queue)
	}

	// Cycle breaking logic:
	// If cyclical dependencies exist, break cycles gracefully using kind ranking.
	if len(sorted) < len(ids) {
		if proc != nil && proc.Logger() != nil {
			logging.FluentEvent(proc.Logger()).Warn("cyclical dependency detected during membrane promotion; breaking cycles gracefully").Log()
		}

		for len(sorted) < len(ids) {
			var remaining []string
			for _, id := range ids {
				if !visited[id] {
					remaining = append(remaining, id)
				}
			}
			if len(remaining) == 0 {
				break
			}

			sort.SliceStable(remaining, func(i, j int) bool {
				idA, idB := remaining[i], remaining[j]
				rankA := kindPromotionRank(kindsMap[idA])
				rankB := kindPromotionRank(kindsMap[idB])
				if rankA != rankB {
					return rankA < rankB
				}
				if inDegree[idA] != inDegree[idB] {
					return inDegree[idA] < inDegree[idB]
				}
				return origIndex[idA] < origIndex[idB]
			})

			best := remaining[0]
			visited[best] = true
			sorted = append(sorted, best)

			for _, dep := range dependents[best] {
				inDegree[dep]--
				if inDegree[dep] == 0 && !visited[dep] {
					queue = append(queue, dep)
				}
			}
			sortQueue(queue)

			for len(queue) > 0 {
				curr := queue[0]
				queue = queue[1:]
				if visited[curr] {
					continue
				}
				visited[curr] = true
				sorted = append(sorted, curr)

				for _, dep := range dependents[curr] {
					inDegree[dep]--
					if inDegree[dep] == 0 && !visited[dep] {
						queue = append(queue, dep)
					}
				}
				sortQueue(queue)
			}
		}
	}

	return sorted, nil
}

// preparePromoteTargetObjectIDs handles argument expansion, optional cascade discovery,
// and topological sorting before executing membrane promotions.
func preparePromoteTargetObjectIDs(cmd *cobra.Command, proc *cli.Processor, rawArgs []string) ([]string, error) {
	if proc != nil && proc.OperationContext() != nil && proc.OperationContext().Err() != nil {
		return nil, proc.OperationContext().Err()
	}
	if cmd != nil && cmd.Context() != nil && cmd.Context().Err() != nil {
		return nil, cmd.Context().Err()
	}

	args, err := parseTargetObjectIDs(cmd, rawArgs)
	if err != nil {
		return nil, err
	}

	ctx := proc.OperationContext()
	secCtx := proc.SecurityContext()

	if isCascadeEnabled(cmd) {
		cascaded, err := expandCascadeDraftDependencies(ctx, secCtx, proc, args)
		if err != nil {
			return nil, err
		}
		args = cascaded
	}

	sorted, err := sortObjectIDsForMembranePromotion(ctx, secCtx, proc, args)
	if err != nil {
		return nil, err
	}

	// Clear --ids flag after consuming it into the sorted slice so that downstream
	// parseTargetObjectIDs calls in executeLifecycleTransitions do not re-append it.
	if cmd != nil && cmd.Flags() != nil && cmd.Flags().Lookup("ids") != nil && cmd.Flags().Changed("ids") {
		_ = cmd.Flags().Set("ids", "")
	}

	return sorted, nil
}
