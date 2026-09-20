package objects

import (
	"sort"
	"strings"
)

// PickTopLevelFields returns a new map containing only the listed top-level keys that exist in src.
// Duplicate keys in the list are ignored after the first occurrence. Empty or whitespace keys are skipped.
// Use after storage has returned the **materialized** object (CAS YAML + runtime_delta overlay + kind), so
// projection does not bypass overlay merge — it only reduces what callers marshal or forward.
func PickTopLevelFields(src map[string]any, keys []string) map[string]any {
	if src == nil || len(keys) == 0 {
		return nil
	}
	out := make(map[string]any)
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		if v, ok := src[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// PickDotPaths selects values reachable via dot-separated paths through nested maps (jq-like **field pick**, not expressions).
// Example: []string{"id", "metadata.version"} copies src["id"] and builds out["metadata"]["version"] when src["metadata"] is a map.
// Paths are applied **longest-first** so deeper picks run before shallower keys that might otherwise overwrite nested maps.
// Non-map segments or missing keys cause that path to be skipped. Array indices are **not** supported in v1.
//
// Avoid mixing a parent path with a child of the same subtree in one call (e.g. both "metadata" and "metadata.x");
// behavior depends on ordering; prefer picking either the whole top-level key or dot paths under it.
func PickDotPaths(src map[string]any, paths []string) map[string]any {
	if src == nil || len(paths) == 0 {
		return nil
	}
	clean := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return nil
	}
	sort.Slice(clean, func(i, j int) bool {
		// Longer paths first (more dots) so nested structure is set before a shorter key overwrites.
		ci := strings.Count(clean[i], ".")
		cj := strings.Count(clean[j], ".")
		if ci != cj {
			return ci > cj
		}
		return clean[i] < clean[j]
	})
	out := make(map[string]any)
	for _, p := range clean {
		parts := strings.Split(p, ".")
		val, ok := getDotPath(src, parts)
		if !ok {
			continue
		}
		setDotPath(out, parts, val)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func getDotPath(m map[string]any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return nil, false
	}
	var cur any = m
	for i, part := range parts {
		if part == "" {
			return nil, false
		}
		if i == len(parts)-1 {
			switch mm := cur.(type) {
			case map[string]any:
				v, ok := mm[part]
				return v, ok
			default:
				return nil, false
			}
		}
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		nxt, ok := mm[part]
		if !ok {
			return nil, false
		}
		cur = nxt
	}
	return nil, false
}

func setDotPath(dst map[string]any, parts []string, val any) {
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		dst[parts[0]] = val
		return
	}
	key := parts[0]
	rest := parts[1:]
	sub, ok := dst[key].(map[string]any)
	if !ok || sub == nil {
		sub = make(map[string]any)
		dst[key] = sub
	}
	setDotPath(sub, rest, val)
}
