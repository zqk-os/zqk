package internal

import (
	"fmt"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// calculatePerKindLimit calculates the limit per kind for listing all objects
func calculatePerKindLimit(storageCtx *pkgctx.StorageContext) int {
	if storageCtx.MaxPageSize > 0 {
		return storageCtx.MaxPageSize
	}
	if storageCtx.DefaultPageSize > 0 {
		return storageCtx.DefaultPageSize
	}
	// Default to 10 items per kind if no limit is set
	return 10
}

// addLifecycleResults adds lifecycle definition results to the collection
func addLifecycleResults(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, builtInOnly, internalOnly, allObjects bool, allResults []map[string]any, kindCounts map[string]int, kindTruncated map[string]bool, perKindLimit int) []map[string]any {
	lifecycleResult, err := listLifecycleDefinitionsForAll(cmd, proc, storageProvider, projectRoot, builtInOnly, internalOnly, allObjects)
	if err != nil || lifecycleResult == nil || len(lifecycleResult.Objects) == 0 {
		return allResults
	}

	totalLifecycles := len(lifecycleResult.Objects)
	kindCounts[internalKindLifecycle] = totalLifecycles

	displayCount := totalLifecycles
	if perKindLimit > 0 && totalLifecycles > perKindLimit {
		displayCount = perKindLimit
		kindTruncated[internalKindLifecycle] = true
	}

	for i := 0; i < displayCount && i < len(lifecycleResult.Objects); i++ {
		allResults = append(allResults, lifecycleResult.Objects[i])
	}

	return allResults
}

// addSpecResults adds object spec results to the collection
func addSpecResults(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, builtInOnly, internalOnly, allObjects bool, allResults []map[string]any, kindCounts map[string]int, kindTruncated map[string]bool, perKindLimit int) []map[string]any {
	specResult, err := listObjectSpecsForAll(cmd, proc, storageProvider, projectRoot, builtInOnly, internalOnly, allObjects)
	if err != nil || specResult == nil || len(specResult.Objects) == 0 {
		return allResults
	}

	totalSpecs := len(specResult.Objects)
	kindCounts[internalKindObjectSpec] = totalSpecs

	displayCount := totalSpecs
	if perKindLimit > 0 && totalSpecs > perKindLimit {
		displayCount = perKindLimit
		kindTruncated[internalKindObjectSpec] = true
	}

	for i := 0; i < displayCount && i < len(specResult.Objects); i++ {
		allResults = append(allResults, specResult.Objects[i])
	}

	return allResults
}

// shouldIncludeObject determines if an object should be included based on flags
func shouldIncludeObject(obj map[string]any, builtInOnly, internalOnly, allObjects bool) bool {
	if allObjects {
		return true
	}
	if builtInOnly {
		return storage.IsBuiltIn(obj)
	}
	if internalOnly {
		return isInternalObject(obj)
	}
	// Default: include built-in or internal objects
	return storage.IsBuiltIn(obj) || isInternalObject(obj)
}

// filterObjectsByFlags filters objects based on builtInOnly, internalOnly, allObjects flags
func filterObjectsByFlags(objects []map[string]any, builtInOnly, internalOnly, allObjects bool) []map[string]any {
	filtered := make([]map[string]any, 0)
	for _, obj := range objects {
		if shouldIncludeObject(obj, builtInOnly, internalOnly, allObjects) {
			filtered = append(filtered, obj)
		}
	}
	return filtered
}

// addKindResults adds results for a specific kind to the collection.
// Uses Count() for the per-kind total so "X shown of Y total" is accurate for high-volume kinds
// (e.g. audit_event); otherwise len(List()) would cap at a small number and show a false total.
func addKindResults(proc *cli.Processor, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, builtInOnly, internalOnly, allObjects bool, allResults []map[string]any, kindCounts map[string]int, kindTruncated map[string]bool, perKindLimit int) []map[string]any {
	listFilter := storage.ListFilter{
		Kind:    kind,
		Filters: make(map[string]any),
	}
	// Request only perKindLimit items so we don't load 250k+ for audit_event
	effectiveLimit := perKindLimit
	if effectiveLimit <= 0 {
		effectiveLimit = 50
	}
	listFilter.Limit = effectiveLimit

	result, err := storageProvider.List(proc.OperationContext(), secCtx, storageCtx, listFilter)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error(fmt.Sprintf("Failed to list %s objects", kind), err).
			String("kind", kind).
			Log()
		return allResults // Continue with other kinds
	}

	// Filter based on flags
	filtered := filterObjectsByFlags(result.Objects, builtInOnly, internalOnly, allObjects)

	// Total count from Count() so displayed total is accurate (object-count-report alignment)
	totalCount, countErr := storageProvider.Count(proc.OperationContext(), secCtx, storage.ListFilter{Kind: kind})
	if countErr != nil {
		totalCount = len(filtered) // Fallback to list length
	}
	kindCounts[kind] = totalCount

	// Limit displayed results per kind
	displayCount := len(filtered)
	if totalCount > effectiveLimit {
		kindTruncated[kind] = true
	}

	for i := 0; i < displayCount && i < len(filtered); i++ {
		allResults = append(allResults, filtered[i])
	}

	return allResults
}

// buildGroupedResult builds a QueryResult with default grouping by kind
func buildGroupedResult(allResults []map[string]any, groupBy string, idsOnly bool) *storage.QueryResult {
	result := &storage.QueryResult{
		Objects: allResults,
		Meta:    map[string]any{"total_count": len(allResults)},
	}

	// Apply grouping if requested
	if groupBy != emptyValue {
		result = groupResults(result, groupBy)
	} else if !idsOnly {
		// Default grouping by kind for table output
		grouped := make(map[string][]map[string]any)
		for _, obj := range allResults {
			objKind, _ := obj[objects.FieldKeyKind].(string)
			if objKind == emptyValue {
				objKind = "unknown"
			}
			grouped[objKind] = append(grouped[objKind], obj)
		}
		result.Groups = grouped
		result.Meta["total_groups"] = len(grouped)
	}

	return result
}

// outputAllInternalResults outputs results for all internal objects
func outputAllInternalResults(cmd *cobra.Command, proc *cli.Processor, result *storage.QueryResult, allResults []map[string]any, kindCounts map[string]int, kindTruncated map[string]bool, builtInOnly, internalOnly, allObjects bool, groupBy string, idsOnly bool) error {
	format := proc.Format()
	if idsOnly {
		outputListIDsOnly(cmd, result, format, "")
		return nil
	}
	if groupBy != emptyValue || format == cli.FormatJSON || format == cli.FormatYAML {
		// For structured formats or when grouping is requested, use outputList
		outputList(cmd, result, format, "", false)
		return nil
	}
	// For table format without grouping, show grouped by kind using custom table
	outputAllInternalTable(cmd, allResults, kindCounts, kindTruncated, builtInOnly, internalOnly, allObjects)
	return nil
}
