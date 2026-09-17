package system

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

// discoverObjectsByIDs discovers objects by their IDs
func discoverObjectsByIDs(ctx context.Context, storageProvider storage.ObjectStorageProvider, objectIDs []string, logger logging.Logger) []string {
	if len(objectIDs) == 0 {
		return nil
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	var allObjectIDs []string

	for _, objectID := range objectIDs {
		exists, err := storageProvider.Exists(ctx, secCtx, objectID)
		if err != nil {
			logging.Fluent(logger).Warn("Failed to check object existence").
				String("object_id", objectID).
				WithError(err).
				Log()
			continue
		}
		if exists {
			allObjectIDs = append(allObjectIDs, objectID)
		} else {
			logging.Fluent(logger).Warn("Object not found, skipping").
				String("object_id", objectID).
				Log()
		}
	}

	return allObjectIDs
}

// checkSnapableTrait checks if a spec has the snapable trait
func checkSnapableTrait(spec *objects.Spec) bool {
	if spec == nil {
		return false
	}
	for _, trait := range spec.Traits {
		if trait == "snapable" {
			return true
		}
	}
	return false
}

// buildListFilterFromPreset builds a ListFilter from a query preset map
func buildListFilterFromPreset(kind string, presetQueryMap map[string]any) *storage.ListFilter {
	listFilter := &storage.ListFilter{
		Kind:    kind,
		Filters: make(map[string]any),
	}

	if filters, ok := presetQueryMap["filters"].(map[string]any); ok {
		listFilter.Filters = filters
	}
	if sortBy, ok := presetQueryMap["sort_by"].(string); ok {
		listFilter.SortBy = sortBy
	}
	if sortAsc, ok := presetQueryMap["sort_asc"].(bool); ok {
		listFilter.SortAsc = sortAsc
	}
	if limit, ok := presetQueryMap["limit"].(int); ok {
		listFilter.Limit = limit
	}

	return listFilter
}

// buildListFilterFromDefault builds a ListFilter from default query map
func buildListFilterFromDefault(kind string, defaultQuery map[string]any) *storage.ListFilter {
	listFilter := &storage.ListFilter{
		Kind:    kind,
		Filters: make(map[string]any),
	}

	if filters, ok := defaultQuery["filters"].(map[string]any); ok {
		listFilter.Filters = filters
	}
	if sortBy, ok := defaultQuery["sort_by"].(string); ok {
		listFilter.SortBy = sortBy
	}
	if sortAsc, ok := defaultQuery["sort_asc"].(bool); ok {
		listFilter.SortAsc = sortAsc
	}
	if limit, ok := defaultQuery["limit"].(int); ok {
		listFilter.Limit = limit
	}

	return listFilter
}

// getSnapableQueryFilter gets the query filter from snapable trait configuration
func getSnapableQueryFilter(kind string, queryPreset string, specLoader *objects.SpecLoader, traitRegistry *objects.TraitRegistry, logger logging.Logger) *storage.ListFilter {
	spec, err := specLoader.LoadSpecWithInheritance(kind)
	if err != nil {
		return nil
	}

	if !checkSnapableTrait(spec) {
		return nil
	}

	objQuery, err := objects.GetSnapableObjectQuery(traitRegistry)
	if err != nil || objQuery == nil {
		return nil
	}

	// Use query preset if specified
	if queryPreset != emptyValue {
		if presetQueryMap, ok := objQuery.QueryPresets[queryPreset]; ok {
			logging.Fluent(logger).Debug("Using query preset").
				Kind(kind).
				String("preset", queryPreset).
				Log()
			return buildListFilterFromPreset(kind, presetQueryMap)
		}
		logging.Fluent(logger).Warn("Query preset not found, using default").
			Kind(kind).
			String("preset", queryPreset).
			Log()
	}

	// Fall back to default query
	if objQuery.DefaultQuery != nil {
		logging.Fluent(logger).Debug("Using default query from snapable trait").
			Kind(kind).
			Log()
		return buildListFilterFromDefault(kind, objQuery.DefaultQuery)
	}

	return nil
}

// mergeListFilter merges snapable query config into base filter
func mergeListFilter(baseFilter storage.ListFilter, snapableFilter *storage.ListFilter) storage.ListFilter {
	if snapableFilter == nil {
		return baseFilter
	}

	if snapableFilter.Filters != nil {
		baseFilter.Filters = snapableFilter.Filters
	}
	if snapableFilter.SortBy != emptyValue {
		baseFilter.SortBy = snapableFilter.SortBy
		baseFilter.SortAsc = snapableFilter.SortAsc
	}
	if snapableFilter.Limit > 0 {
		baseFilter.Limit = snapableFilter.Limit
	}

	return baseFilter
}

// discoverObjectsByKind discovers objects for a single kind
func discoverObjectsByKind(ctx context.Context, storageProvider storage.ObjectStorageProvider, kind string, queryPreset string, specLoader *objects.SpecLoader, traitRegistry *objects.TraitRegistry, logger logging.Logger) []string {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Get snapable query filter if available
	snapableFilter := getSnapableQueryFilter(kind, queryPreset, specLoader, traitRegistry, logger)

	// Build base filter
	filter := storage.ListFilter{
		Kind:    kind,
		Filters: map[string]any{},
	}

	// Merge snapable config if available
	filter = mergeListFilter(filter, snapableFilter)

	// List objects
	result, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		logging.Fluent(logger).Warn("Failed to list objects").
			Kind(kind).
			WithError(err).
			Log()
		return nil
	}

	// Extract object IDs
	var objectIDs []string
	for _, obj := range result.Objects {
		if id, ok := obj[objects.FieldKeyID].(string); ok {
			objectIDs = append(objectIDs, id)
		}
	}

	return objectIDs
}

// discoverObjectsByKinds discovers objects by their kinds
func discoverObjectsByKinds(ctx context.Context, storageProvider storage.ObjectStorageProvider, kinds []string, queryPreset string, logger logging.Logger) []string {
	if len(kinds) == 0 {
		return nil
	}

	specLoader := objects.GetGlobalSpecLoader()
	traitRegistry := objects.NewTraitRegistry()

	var allObjectIDs []string
	for _, kind := range kinds {
		objectIDs := discoverObjectsByKind(ctx, storageProvider, kind, queryPreset, specLoader, traitRegistry, logger)
		allObjectIDs = append(allObjectIDs, objectIDs...)
	}

	return allObjectIDs
}

// removeDuplicateIDs removes duplicate IDs from a slice
func removeDuplicateIDs(ids []string) []string {
	seen := make(map[string]bool)
	var uniqueIDs []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	return uniqueIDs
}
