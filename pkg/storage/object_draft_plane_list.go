package storage

import "github.com/zqk-os/zqk/pkg/objects"

// omitDraftPlaneOnlyFromList drops objects that exist only on the draft plane
// (.zqk/object_drafts). Dual-plane CAS copies stay listable. Get/Exists still
// follow the identity cache onto draft YAML.
//
// Draft plane is a location, not a lifecycle status. Filters such as
// status=draft or status=conceptual must not surface conceptual/exploring
// objects that have not left the draft plane.
// TRACK: BLI-1786689721908382000-6402a858
func (f *FileObjectStorage) omitDraftPlaneOnlyFromList(result *QueryResult) {
	if result == nil || len(result.Objects) == 0 {
		return
	}
	dropped := make(map[string]struct{})
	origN := len(result.Objects)
	kept := make([]map[string]any, 0, origN)
	for _, obj := range result.Objects {
		id := objects.GetString(obj, objects.FieldKeyID)
		if f.listedObjectIsDraftPlaneOnly(obj) {
			if id != emptyValue {
				dropped[id] = struct{}{}
			}
			continue
		}
		kept = append(kept, obj)
	}
	if len(dropped) == 0 {
		return
	}
	result.Objects = kept
	if result.Groups != nil {
		for g, objs := range result.Groups {
			gkept := make([]map[string]any, 0, len(objs))
			for _, obj := range objs {
				id := objects.GetString(obj, objects.FieldKeyID)
				if _, ok := dropped[id]; ok {
					continue
				}
				gkept = append(gkept, obj)
			}
			result.Groups[g] = gkept
		}
	}
	if result.Meta == nil {
		result.Meta = map[string]any{}
	}
	result.Meta[ConstStreamReturnedCount] = len(kept)
	droppedN := origN - len(kept)
	if v, ok := result.Meta["total_count"].(int); ok && v >= droppedN {
		result.Meta["total_count"] = v - droppedN
	}
}

// listedObjectIsDraftPlaneOnly is true when the id lives on the draft plane and
// is not in the CAS index. CAS-backed (including dual-plane) objects stay listable.
func (f *FileObjectStorage) listedObjectIsDraftPlaneOnly(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	id := objects.GetString(obj, objects.FieldKeyID)
	kind := objects.GetString(obj, objects.FieldKeyKind)
	if id == emptyValue || kind == emptyValue {
		return false
	}
	live := cachedLivePath(id)
	if live != emptyValue && !IsObjectDraftPlanePath(f.projectRoot, live) {
		return false
	}
	onDraft := (live != emptyValue && IsObjectDraftPlanePath(f.projectRoot, live)) || f.objectDraftPlaneExists(kind, id)
	if !onDraft {
		return false
	}
	return !casBackedObjectID(f.projectRoot, kind, id)
}

// dropDraftPlaneOnlyIDs removes IDs that are not in the CAS index and exist
// only as draft-plane YAML (write-behind / explicit-id list leaks).
func (f *FileObjectStorage) dropDraftPlaneOnlyIDs(kind string, idSet, casIDs map[string]bool) {
	if len(idSet) == 0 {
		return
	}
	for id := range idSet {
		if casIDs[id] {
			continue
		}
		live := cachedLivePath(id)
		if live != emptyValue && IsObjectDraftPlanePath(f.projectRoot, live) {
			delete(idSet, id)
			continue
		}
		if f.objectDraftPlaneExists(kind, id) && !casBackedObjectID(f.projectRoot, kind, id) {
			delete(idSet, id)
		}
	}
}
