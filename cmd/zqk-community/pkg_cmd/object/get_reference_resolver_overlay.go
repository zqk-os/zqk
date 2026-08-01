package object

import (
	"fmt"
	"regexp"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/objectget"
	"github.com/lanceman/zqk/pkg/objects"
)

const maxReferenceResolverOverlayReads = 100

var referenceFieldKeyRe = regexp.MustCompile(`_(ref|refs)$`)

// applyReferenceResolverOverlay applies a best-effort "reference resolver overlay" to the object map.
// It resolves reference fields (keys ending with `_ref` / `_refs`) and injects `resolved_<field>` entries
// that include at least id/title/kind/status for each referenced object.
func applyReferenceResolverOverlay(proc *cli.Processor, obj map[string]any, cfg objectget.ReferenceResolverOverlayConfig) {
	cfg = objectget.NormalizeReferenceOverlayConfig(cfg)

	visited := make(map[string]struct{}, cfg.MaxTotalUniqueIDs)
	idMemo := make(map[string]map[string]any, cfg.MaxTotalUniqueIDs) // in-request cache

	stats := &referenceResolverOverlayStats{}

	var resolveSet map[string]struct{}
	if len(cfg.ResolveFieldKeys) > 0 {
		resolveSet = make(map[string]struct{}, len(cfg.ResolveFieldKeys))
		for _, k := range cfg.ResolveFieldKeys {
			resolveSet[k] = struct{}{}
		}
	}

	processed := make(map[string]struct{}, cfg.MaxTotalUniqueIDs) // prevents re-recursing into same referenced ID
	resolveReferenceResolverOverlayRecursive(
		proc,
		obj,
		resolveSet,
		cfg.MaxDepth,
		visited,
		processed,
		idMemo,
		stats,
		cfg.MaxTotalUniqueIDs,
		cfg.MaxTotalResolvedEntry,
	)

	obj["reference_resolver_overlay_applied"] = true
	obj["reference_resolver_overlay_max_depth_reached"] = stats.maxDepthReached
	obj["reference_resolver_overlay_max_total_unique_ids_capped"] = stats.maxTotalUniqueIDsReached
	obj["reference_resolver_overlay_max_total_resolved_entries_capped"] = stats.maxTotalResolvedEntryReached
	obj["reference_resolver_overlay_capped"] = stats.cappedAnything
}

func isReferenceFieldKey(key string) bool {
	// Common reference naming conventions in this codebase:
	// - plural: *_refs (list of IDs)
	// - singular: *_ref  (single ID)
	return referenceFieldKeyRe.MatchString(key)
}

func extractReferenceIDs(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

type referenceResolverOverlayStats struct {
	maxDepthReached              bool
	maxTotalUniqueIDsReached     bool
	maxTotalResolvedEntryReached bool
	cappedAnything               bool
	totalResolvedEntries         int
}

func resolveReferenceResolverOverlayRecursive(
	proc *cli.Processor,
	obj map[string]any,
	resolveSet map[string]struct{},
	depthRemaining int,
	visited map[string]struct{},
	processed map[string]struct{},
	idMemo map[string]map[string]any,
	stats *referenceResolverOverlayStats,
	maxTotalUniqueIDs int,
	maxTotalResolvedEntry int,
) {
	if depthRemaining <= 0 {
		stats.maxDepthReached = true
		return
	}

	// Collect candidate reference fields from keys.
	refFieldIDs := make(map[string][]string)
	for k, v := range obj {
		if !isReferenceFieldKey(k) {
			continue
		}
		if resolveSet != nil {
			if _, ok := resolveSet[k]; !ok {
				continue
			}
		}

		ids := extractReferenceIDs(v)
		if len(ids) == 0 {
			continue
		}

		// Exclude empty strings defensively.
		filtered := ids[:0]
		for _, id := range ids {
			if id != "" {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) == 0 {
			continue
		}

		refFieldIDs[k] = filtered
	}

	if len(refFieldIDs) == 0 {
		return
	}
	if depthRemaining == 1 {
		// We will resolve one layer on this object, but we intentionally stop
		// recursion here due to the configured depth budget.
		stats.maxDepthReached = true
	}

	// De-duplicate referenced IDs across all fields for this layer.
	uniqueIDs := make([]string, 0)
	seen := make(map[string]struct{}, len(refFieldIDs))
	for _, ids := range refFieldIDs {
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	// Avoid unbounded work even in the recursive case.
	// This keeps a single object get fast even if a single object has huge lists.
	if len(uniqueIDs) > maxReferenceResolverOverlayReads {
		uniqueIDs = uniqueIDs[:maxReferenceResolverOverlayReads]
		stats.cappedAnything = true
	}

	// Bulk fetch referenced objects (memoized per request).
	ctx := proc.OperationContext()
	secCtx := proc.SecurityContext()

	toFetch := make([]string, 0)
	for _, id := range uniqueIDs {
		if _, ok := idMemo[id]; ok {
			continue
		}
		toFetch = append(toFetch, id)
	}

	// Apply global unique-id cap for worst-case safety.
	if maxTotalUniqueIDs > 0 && len(visited)+len(toFetch) > maxTotalUniqueIDs {
		stats.maxTotalUniqueIDsReached = true
		stats.cappedAnything = true
		allowed := maxTotalUniqueIDs - len(visited)
		if allowed < 0 {
			allowed = 0
		}
		if allowed < len(toFetch) {
			toFetch = toFetch[:allowed]
		}
	}

	if len(toFetch) > 0 {
		bulk, err := proc.Storage().BulkGet(ctx, secCtx, toFetch)
		if err != nil {
			// Best-effort: don't break `object get` if resolution fails.
			obj["reference_resolver_overlay_error"] = fmt.Sprintf("%v", err)
			return
		}

		for _, entry := range bulk.Results {
			refID, _ := entry[objects.FieldKeyID].(string)
			if refID == "" {
				continue
			}
			idMemo[refID] = entry
			visited[refID] = struct{}{}
		}
	}

	// Inject resolved slices for each reference field.
	entriesCreated := 0
	for fieldName, ids := range refFieldIDs {
		resolvedKey := "resolved_" + fieldName
		resolved := make([]map[string]any, 0, len(ids))

		for _, id := range ids {
			embed := make(map[string]any, 4)
			embed[objects.FieldKeyID] = id

			refObj := idMemo[id]
			if refObj != nil {
				embed[objects.FieldKeyKind] = refObj[objects.FieldKeyKind]
				embed[objects.FieldKeyTitle] = refObj[objects.FieldKeyTitle]
				embed[objects.FieldKeyStatus] = refObj[objects.FieldKeyStatus]
			}

			// For deeper resolution we need the raw reference fields on the referenced object.
			// Copy only when we still have remaining depth budget.
			if refObj != nil && depthRemaining > 1 {
				for k, v := range refObj {
					if !isReferenceFieldKey(k) {
						continue
					}
					if resolveSet != nil {
						if _, ok := resolveSet[k]; !ok {
							continue
						}
					}
					embed[k] = v
				}
			}

			// Attempt deeper resolution for this referenced object.
			if depthRemaining > 1 && !stats.maxTotalResolvedEntryReached && !stats.maxTotalUniqueIDsReached {
				if _, done := processed[id]; !done {
					processed[id] = struct{}{}
					resolveReferenceResolverOverlayRecursive(
						proc,
						embed,
						resolveSet,
						depthRemaining-1,
						visited,
						processed,
						idMemo,
						stats,
						maxTotalUniqueIDs,
						maxTotalResolvedEntry,
					)
				}
			}

			resolved = append(resolved, embed)
			entriesCreated++
			stats.totalResolvedEntries++
			if maxTotalResolvedEntry > 0 && stats.totalResolvedEntries >= maxTotalResolvedEntry {
				stats.maxTotalResolvedEntryReached = true
				stats.cappedAnything = true
			}
		}

		obj[resolvedKey] = resolved
	}
}
