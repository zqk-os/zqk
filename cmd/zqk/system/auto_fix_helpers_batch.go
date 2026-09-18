package system

import (
	"fmt"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// batchFixHashMismatches efficiently fixes multiple hash mismatches in one batch
// This prevents the iterative process where fixing one creates new mismatches
func batchFixHashMismatches(fixCtx *AutoFixContext, issues []Issue, _ storage.HashRegistryProvider) []string {
	if len(issues) == 0 {
		return []string{}
	}

	projectRoot := fixCtx.Ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	if projectRoot == emptyValue {
		logging.Fluent(fixCtx.Logger).Warn("Cannot batch fix hash mismatches: no project root").
			String("object_id", fixCtx.Obj.ID).
			Log()
		return []string{}
	}

	// Group hash mismatches by kind for efficient batch processing
	// All issues are for the same object (fixCtx.Obj.ID), so we just need the kind
	fixKind := determineFixKind(fixCtx)
	if fixKind == emptyValue {
		logging.Fluent(fixCtx.Logger).Warn("Cannot batch fix hash mismatches: unable to determine kind").
			String("object_id", fixCtx.Obj.ID).
			Log()
		return []string{}
	}

	// Since all issues are for the same object, we only need to fix it once
	hashMismatchesByKind := map[string][]string{
		fixKind: {fixCtx.Obj.ID},
	}

	// Fix all hash mismatches by kind in batch
	var fixed []string
	stdctx := pkgctx.NewSystemContext()
	strategy := storage.NewRemoveStaleIndexStrategy(fixCtx.Logger)
	fixer := storage.NewHashMismatchFixer(strategy, fixCtx.Logger)

	for kind, objectIDs := range hashMismatchesByKind {
		// Remove duplicates
		uniqueIDs := make(map[string]bool)
		deduplicatedIDs := make([]string, 0, len(objectIDs))
		for _, id := range objectIDs {
			if !uniqueIDs[id] {
				uniqueIDs[id] = true
				deduplicatedIDs = append(deduplicatedIDs, id)
			}
		}

		if len(deduplicatedIDs) == 0 {
			continue
		}

		logging.Fluent(fixCtx.Logger).Info("Batch fixing hash mismatches").
			Kind(kind).
			Count(len(deduplicatedIDs)).
			Log()

		results, err := fixer.FixHashMismatches(stdctx, deduplicatedIDs, kind, projectRoot)
		if err != nil {
			logging.Fluent(fixCtx.Logger).Warn("Batch hash mismatch fix failed").
				Kind(kind).
				WithError(err).
				Log()
			continue
		}

		// Count successful fixes
		successCount := 0
		for _, result := range results {
			if result.Fixed {
				successCount++
				fixed = append(fixed, fmt.Sprintf("Batch fixed hash mismatch for %s using %s strategy", result.ObjectID, result.Strategy))
			}
		}

		if successCount > 0 {
			logging.Fluent(fixCtx.Logger).Info("Batch fixed hash mismatches").
				Kind(kind).
				Int("fixed_count", successCount).
				Int("total_count", len(results)).
				Log()
		}
	}

	return fixed
}
