package storage

import "maps"

// asStringKeyedMap returns v as map[string]any when v is a YAML/JSON object shape.
// yaml.v3 may decode maps as map[string]any or map[any]any depending on nesting.
func asStringKeyedMap(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				return nil, false
			}
			out[ks] = val
		}
		return out, true
	default:
		return nil, false
	}
}

// applyFieldUpdates merges updates into existing. FieldUnset deletes the key instead of writing a value.
// Write-behind WAL batch prepare must use this — assigning FieldUnset and yaml.Marshal'ing it persists `{}`,
// which still trips occupancy OpRefuseFieldPresent (key present, including empty maps).
func applyFieldUpdates(existing map[string]any, updates map[string]any) {
	if existing == nil || len(updates) == 0 {
		return
	}
	for k, v := range updates {
		if IsFieldUnset(v) {
			delete(existing, k)
			continue
		}
		if mergeMapPatchIntoExisting(existing, k, v) {
			continue
		}
		existing[k] = v
	}
}

// mergeMapPatchIntoExisting shallow-merges patch into existing[field] when patch is object-shaped.
// Patch keys win on collision. If the existing value is not a map, it is replaced by a copy of patch
// (same as assigning a new map). Returns false when patch is not a map — caller should assign patch directly.
//
// This fixes dotted CLI updates (e.g. notes.Foo=bar) and partial --file payloads that only set a subset
// of keys under a map field, without wiping sibling keys.
func mergeMapPatchIntoExisting(existing map[string]any, field string, patch any) bool {
	patchMap, ok := asStringKeyedMap(patch)
	if !ok {
		return false
	}
	baseVal, hasBase := existing[field]
	baseMap, baseOk := asStringKeyedMap(baseVal)
	if !hasBase || !baseOk {
		out := make(map[string]any, len(patchMap))
		maps.Copy(out, patchMap)
		existing[field] = out
		return true
	}
	merged := make(map[string]any, len(baseMap)+len(patchMap))
	maps.Copy(merged, baseMap)
	maps.Copy(merged, patchMap)
	existing[field] = merged
	return true
}
