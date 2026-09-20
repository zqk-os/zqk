package storage

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestComprehensiveFilteringSortingGrouping tests filtering, sorting, and grouping
// for all discoverable object kinds with permutations of all available fields.
// Skip with -short for faster feedback.
func TestComprehensiveFilteringSortingGrouping(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive filtering/sorting/grouping in short mode")
	}
	testRoot, storage, secCtx := setupTestingFactoryCompleteTestEnvironment(t)

	// Stream metrics (command_metric, audit_aggregation_metric, etc.) resolve segment dirs via path alias cache.
	BuildPathAliasCacheForProject(testRoot)

	ctx := context.Background()
	storageCtx := pkgctx.GetStorageContext()

	// Get all discoverable object kinds
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	kinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		t.Fatalf("failed to get all kinds: %v", err)
	}

	// For each kind, test comprehensive operations
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			testKindComprehensive(t, storage, ctx, secCtx, storageCtx, kind, fieldRegistry)
		})
	}
}

// testKindComprehensive tests all filtering, sorting, and grouping operations for a specific kind
func testKindComprehensive(t *testing.T, storage *FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, fieldRegistry *objects.FieldRegistry) {
	// Get field information for this kind
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		t.Logf("skipping %s: failed to get fields: %v", kind, err)
		return
	}

	// Get filterable, sortable, and groupable fields
	filterableFields, err := objects.GenerateFilterableFields(kind)
	if err != nil {
		t.Logf("skipping %s: failed to get filterable fields: %v", kind, err)
		return
	}

	sortableFields, err := objects.GenerateSortableFields(kind)
	if err != nil {
		t.Logf("skipping %s: failed to get sortable fields: %v", kind, err)
		return
	}

	groupableFields, err := objects.GenerateGroupableFields(kind)
	if err != nil {
		t.Logf("skipping %s: failed to get groupable fields: %v", kind, err)
		return
	}

	// First, create some test objects for this kind
	testObjects := createTestObjectsForKind(t, storage, ctx, secCtx, kind, kindFields)
	if len(testObjects) == 0 {
		t.Logf("skipping %s: no test objects created", kind)
		return
	}

	// Test 1: Filtering on each filterable field
	t.Run("Filtering", func(t *testing.T) {
		testFilteringPermutations(t, storage, ctx, secCtx, storageCtx, kind, filterableFields, testObjects)
	})

	// Test 2: Sorting on each sortable field
	t.Run("Sorting", func(t *testing.T) {
		testSortingPermutations(t, storage, ctx, secCtx, storageCtx, kind, sortableFields, testObjects)
	})

	// Test 3: Grouping on each groupable field
	t.Run("Grouping", func(t *testing.T) {
		testGroupingPermutations(t, storage, ctx, secCtx, storageCtx, kind, groupableFields, testObjects)
	})

	// Test 4: Multi-hop scenarios (combinations)
	t.Run("MultiHop", func(t *testing.T) {
		testMultiHopScenarios(t, storage, ctx, secCtx, storageCtx, kind, filterableFields, sortableFields, groupableFields, testObjects)
	})
}

// testFilteringPermutations tests filtering on all filterable fields with various operators
func testFilteringPermutations(t *testing.T, storage *FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, filterableFields []string, testObjects []map[string]any) {
	if len(filterableFields) == 0 {
		t.Skip("no filterable fields")
	}

	for _, field := range filterableFields {
		t.Run(field, func(t *testing.T) {
			// Test equality filter
			if len(testObjects) > 0 {
				// Get a sample value from test objects
				sampleValue := getFieldValue(testObjects[0], field)
				if sampleValue != nil {
					filters := map[string]any{field: sampleValue}
					result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
						Kind:    kind,
						Filters: filters,
					})
					if err != nil {
						t.Errorf("filter %s=%v failed: %v", field, sampleValue, err)
					} else {
						t.Logf("filter %s=%v returned %d objects", field, sampleValue, len(result.Objects))
					}
				}
			}

			// Test not equal filter
			if len(testObjects) > 0 {
				sampleValue := getFieldValue(testObjects[0], field)
				if sampleValue != nil {
					filters := map[string]any{
						field: map[string]any{"$ne": sampleValue},
					}
					result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
						Kind:    kind,
						Filters: filters,
					})
					if err != nil {
						t.Errorf("filter %s!=%v failed: %v", field, sampleValue, err)
					} else {
						t.Logf("filter %s!=%v returned %d objects", field, sampleValue, len(result.Objects))
					}
				}
			}
		})
	}
}

// testSortingPermutations tests sorting on all sortable fields
func testSortingPermutations(t *testing.T, storage *FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, sortableFields []string, _ []map[string]any) {
	if len(sortableFields) == 0 {
		t.Skip("no sortable fields")
	}

	for _, field := range sortableFields {
		t.Run(field, func(t *testing.T) {
			// Test ascending sort
			result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
				Kind:    kind,
				SortBy:  field,
				SortAsc: true,
			})
			if err != nil {
				t.Errorf("sort %s ASC failed: %v", field, err)
			} else {
				t.Logf("sort %s ASC returned %d objects", field, len(result.Objects))
				// Verify sort order
				verifySortOrder(t, result.Objects, field, true)
			}

			// Test descending sort
			result, err = storage.List(ctx, secCtx, storageCtx, ListFilter{
				Kind:    kind,
				SortBy:  field,
				SortAsc: false,
			})
			if err != nil {
				t.Errorf("sort %s DESC failed: %v", field, err)
			} else {
				t.Logf("sort %s DESC returned %d objects", field, len(result.Objects))
				// Verify sort order
				verifySortOrder(t, result.Objects, field, false)
			}
		})
	}
}

// testGroupingPermutations tests grouping on all groupable fields
func testGroupingPermutations(t *testing.T, storage *FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, groupableFields []string, _ []map[string]any) {
	if len(groupableFields) == 0 {
		t.Skip("no groupable fields")
	}

	storageCtx.EnableGrouping = true

	for _, field := range groupableFields {
		t.Run(field, func(t *testing.T) {
			result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
				Kind:    kind,
				GroupBy: field,
			})
			if err != nil {
				t.Errorf("group by %s failed: %v", field, err)
			} else {
				t.Logf("group by %s returned %d groups", field, len(result.Groups))
				// Verify grouping
				verifyGrouping(t, result.Groups, field)
			}
		})
	}
}

// testMultiHopScenarios tests combinations of filtering, sorting, and grouping
func testMultiHopScenarios(t *testing.T, storage *FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, filterableFields, sortableFields, groupableFields []string, testObjects []map[string]any) {
	storageCtx.EnableGrouping = true

	// Scenario 1: Filter + Sort
	if len(filterableFields) > 0 && len(sortableFields) > 0 {
		t.Run("Filter+Sort", func(t *testing.T) {
			field := filterableFields[0]
			sortField := sortableFields[0]
			sampleValue := getFieldValue(testObjects[0], field)

			if sampleValue != nil {
				result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
					Kind:    kind,
					Filters: map[string]any{field: sampleValue},
					SortBy:  sortField,
					SortAsc: true,
				})
				if err != nil {
					t.Errorf("filter+sort failed: %v", err)
				} else {
					t.Logf("filter %s=%v + sort %s returned %d objects", field, sampleValue, sortField, len(result.Objects))
					verifySortOrder(t, result.Objects, sortField, true)
				}
			}
		})
	}

	// Scenario 2: Filter + Group
	if len(filterableFields) > 0 && len(groupableFields) > 0 {
		t.Run("Filter+Group", func(t *testing.T) {
			field := filterableFields[0]
			groupField := groupableFields[0]
			sampleValue := getFieldValue(testObjects[0], field)

			if sampleValue != nil {
				result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
					Kind:    kind,
					Filters: map[string]any{field: sampleValue},
					GroupBy: groupField,
				})
				if err != nil {
					t.Errorf("filter+group failed: %v", err)
				} else {
					t.Logf("filter %s=%v + group %s returned %d groups", field, sampleValue, groupField, len(result.Groups))
					for groupKey, objects := range result.Groups {
						verifyGrouping(t, map[string][]map[string]any{groupKey: objects}, groupField)
					}
				}
			}
		})
	}

	// Scenario 3: Sort + Group
	if len(sortableFields) > 0 && len(groupableFields) > 0 {
		t.Run("Sort+Group", func(t *testing.T) {
			sortField := sortableFields[0]
			groupField := groupableFields[0]

			result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
				Kind:    kind,
				SortBy:  sortField,
				SortAsc: true,
				GroupBy: groupField,
			})
			if err != nil {
				t.Errorf("sort+group failed: %v", err)
			} else {
				t.Logf("sort %s + group %s returned %d groups", sortField, groupField, len(result.Groups))
				for groupKey, objects := range result.Groups {
					verifySortOrder(t, objects, sortField, true)
					verifyGrouping(t, map[string][]map[string]any{groupKey: objects}, groupField)
				}
			}
		})
	}

	// Scenario 4: Filter + Sort + Group
	if len(filterableFields) > 0 && len(sortableFields) > 0 && len(groupableFields) > 0 {
		t.Run("Filter+Sort+Group", func(t *testing.T) {
			field := filterableFields[0]
			sortField := sortableFields[0]
			groupField := groupableFields[0]
			sampleValue := getFieldValue(testObjects[0], field)

			if sampleValue != nil {
				result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
					Kind:    kind,
					Filters: map[string]any{field: sampleValue},
					SortBy:  sortField,
					SortAsc: true,
					GroupBy: groupField,
				})
				if err != nil {
					t.Errorf("filter+sort+group failed: %v", err)
				} else {
					t.Logf("filter %s=%v + sort %s + group %s returned %d groups", field, sampleValue, sortField, groupField, len(result.Groups))
					for groupKey, objects := range result.Groups {
						verifySortOrder(t, objects, sortField, true)
						verifyGrouping(t, map[string][]map[string]any{groupKey: objects}, groupField)
					}
				}
			}
		})
	}

	// Scenario 5: Multiple filters (AND logic)
	if len(filterableFields) >= 2 {
		t.Run("MultipleFilters", func(t *testing.T) {
			field1 := filterableFields[0]
			field2 := filterableFields[1]
			value1 := getFieldValue(testObjects[0], field1)
			value2 := getFieldValue(testObjects[0], field2)

			if value1 != nil && value2 != nil {
				result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
					Kind: kind,
					Filters: map[string]any{
						field1: value1,
						field2: value2,
					},
				})
				if err != nil {
					t.Errorf("multiple filters failed: %v", err)
				} else {
					t.Logf("filter %s=%v AND %s=%v returned %d objects", field1, value1, field2, value2, len(result.Objects))
					// Verify all objects match both filters
					for _, obj := range result.Objects {
						if getFieldValue(obj, field1) != value1 {
							t.Errorf("object %v does not match filter %s=%v", obj[objects.FieldKeyID], field1, value1)
						}
						if getFieldValue(obj, field2) != value2 {
							t.Errorf("object %v does not match filter %s=%v", obj[objects.FieldKeyID], field2, value2)
						}
					}
				}
			}
		})
	}

	// Scenario 6: Filter with $in operator
	if len(filterableFields) > 0 {
		t.Run("FilterIn", func(t *testing.T) {
			field := filterableFields[0]
			values := []any{}
			for i := 0; i < minTwo(3, len(testObjects)); i++ {
				val := getFieldValue(testObjects[i], field)
				if val != nil {
					values = append(values, val)
				}
			}

			if len(values) > 0 {
				result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
					Kind: kind,
					Filters: map[string]any{
						field: map[string]any{"$in": values},
					},
				})
				if err != nil {
					t.Errorf("filter %s $in failed: %v", field, err)
				} else {
					t.Logf("filter %s $in %v returned %d objects", field, values, len(result.Objects))
				}
			}
		})
	}

	// Scenario 7: Filter with $nin operator
	if len(filterableFields) > 0 {
		t.Run("FilterNotIn", func(t *testing.T) {
			field := filterableFields[0]
			values := []any{}
			for i := 0; i < minTwo(2, len(testObjects)); i++ {
				val := getFieldValue(testObjects[i], field)
				if val != nil {
					values = append(values, val)
				}
			}

			if len(values) > 0 {
				result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
					Kind: kind,
					Filters: map[string]any{
						field: map[string]any{"$nin": values},
					},
				})
				if err != nil {
					t.Errorf("filter %s $nin failed: %v", field, err)
				} else {
					t.Logf("filter %s $nin %v returned %d objects", field, values, len(result.Objects))
				}
			}
		})
	}
}
