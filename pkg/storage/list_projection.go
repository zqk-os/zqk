package storage

import "github.com/zqk-os/zqk/pkg/objects"

// listProjectionMask builds the hybrid mask for a list query, unioning projected fields with sort and group-by columns.
func listProjectionMask(kind string, filter ListFilter) objects.HybridProjectionMask {
	if len(filter.Fields) == 0 {
		return objects.HybridProjectionMask{}
	}
	names := objects.ListProjectionFieldNames(filter.Fields, filter.SortBy)
	names = objects.ListProjectionFieldNames(names, filter.GroupBy)
	return objects.HybridMaskFromFields(kind, names)
}

// applyListFieldProjection applies hybrid projection after list materialization (filters, sort, overlays).
// Empty filter.Fields leaves objs unchanged (same slice reference).
func applyListFieldProjection(kind string, filter ListFilter, objs []map[string]any) []map[string]any {
	if len(filter.Fields) == 0 || len(objs) == 0 {
		return objs
	}
	mask := listProjectionMask(kind, filter)
	out := make([]map[string]any, len(objs))
	for i := range objs {
		out[i] = objects.ProjectMapHybrid(objs[i], mask)
	}
	return out
}

// projectQueryResultGroups applies the same mask as [applyListFieldProjection] to grouped slices so metadata matches flat Objects.
func projectQueryResultGroups(kind string, filter ListFilter, result *QueryResult) {
	if len(filter.Fields) == 0 || result == nil || len(result.Groups) == 0 {
		return
	}
	mask := listProjectionMask(kind, filter)
	for k, objs := range result.Groups {
		out := make([]map[string]any, len(objs))
		for i := range objs {
			out[i] = objects.ProjectMapHybrid(objs[i], mask)
		}
		result.Groups[k] = out
	}
}
