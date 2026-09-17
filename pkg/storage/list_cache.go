package storage

import (
	"encoding/json"
	"maps"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// mcpListCacheTTL is how long a list result stays reusable inside the MCP
// daemon. Zero-TTL (always miss) was the 2026-09-02 thread-bomb amplifier.
// TRACK: BLI-CEF-STORAGE-INDEX-CACHE-001
const mcpListCacheTTL = 2 * time.Second

// listCacheEntry holds a cached list result (copy of QueryResult)

type listCacheEntry struct {
	Result  *QueryResult
	AddedAt time.Time // used for eviction when at cap (oldest-first)
}

// listCacheMiss reports whether the map lookup did not yield a usable cached list entry.
func listCacheMiss(ok bool, entry *listCacheEntry) bool {
	return !ok || entry == nil || entry.Result == nil
}

// maxListCacheEntries caps list cache size so it cannot grow unbounded (SCHEDULER_MEMORY_LEAK_INVESTIGATION).
const maxListCacheEntries = 1000

// listCache is a process-wide cache for List() results, keyed by projectRoot + list filter.
// It must be invalidated whenever the object ID cache is updated or invalidated, or whenever
// storage is modified (create/update/delete), so list results stay consistent with the ID
// cache and on-disk state. Call InvalidateListCache() for a full clear, or
// InvalidateListCacheForKind(kind) when only one kind has changed. Object ID cache
// invalidation (e.g. from cmd/zqk/system/check_cache.go after create/update/delete) typically
// triggers list cache invalidation.
type listCache struct {
	mu                 sync.RWMutex
	entries            map[string]*listCacheEntry
	hitsTotal          atomic.Int64
	missesTotal        atomic.Int64
	invalidationsTotal atomic.Int64
}

var (
	globalListCache = &listCache{entries: make(map[string]*listCacheEntry)}
)

// GetListCacheStats returns lifetime counters for hits, misses, and invalidations.
func GetListCacheStats() (hits, misses, invalidations int64) {
	return globalListCache.hitsTotal.Load(), globalListCache.missesTotal.Load(), globalListCache.invalidationsTotal.Load()
}

// listCacheKey builds a deterministic cache key from projectRoot, ListFilter, and effectiveLimit.
// effectiveLimit is the limit actually applied (after storageCtx.MaxPageSize/DefaultPageSize).
// Key format: projectRoot|kind|filterHash|sortBy|sortAsc|offset|effectiveLimit|groupBy|fieldsKey
// so InvalidateKind(kind) can remove all entries for that kind.
func listCacheKey(projectRoot string, filter *ListFilter, effectiveLimit int) string {
	filterHash := listFilterHash(filter)
	return strings.Join([]string{
		projectRoot,
		filter.Kind,
		filterHash,
		filter.SortBy,
		strconv.FormatBool(filter.SortAsc),
		strconv.Itoa(filter.Offset),
		strconv.Itoa(effectiveLimit),
		filter.GroupBy,
		listFilterFieldsKey(filter),
	}, "|")
}

// listFilterFieldsKey isolates list cache entries when projected field sets differ.
func listFilterFieldsKey(filter *ListFilter) string {
	if filter == nil || len(filter.Fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(filter.Fields))
	for _, f := range filter.Fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		keys = append(keys, f)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// listFilterHash produces a deterministic string for Filters map (sorted keys).
func listFilterHash(filter *ListFilter) string {
	if filter == nil || len(filter.Filters) == 0 {
		return ""
	}
	keys := make([]string, 0, len(filter.Filters))
	for k := range filter.Filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := filter.Filters[k]
		b.WriteString(k)
		b.WriteString("=")
		// Marshal value for nested maps (e.g. operator syntax)
		if m, ok := v.(map[string]any); ok {
			// Deterministic nested map
			innerKeys := make([]string, 0, len(m))
			for ik := range m {
				innerKeys = append(innerKeys, ik)
			}
			sort.Strings(innerKeys)
			for _, ik := range innerKeys {
				b.WriteString(ik)
				b.WriteString(":")
				b.WriteString(canonicalJSON(m[ik]))
			}
		} else {
			b.WriteString(canonicalJSON(v))
		}
	}
	return b.String()
}

func canonicalJSON(v any) string {
	if v == nil {
		return "null"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

// GetListCache returns a copy of the cached list result if present.
// IsMCPMode returns true if the current execution is running in MCP mode.
func IsMCPMode() bool {
	// Check if running MCP server or inside MCP subprocess
	for _, arg := range os.Args {
		if arg == "mcp" {
			return true
		}
	}
	// Also check env var set in MCP subprocess environment
	mcpAccVar := zqkenv.MCPAccountID()
	if mcpAccVar.Get() != "" {
		return true
	}
	// Legacy fallback
	if zqkenv.MCPAccountID().Get() != "" {
		return true
	}
	return false
}

// GetListCache returns a copy of the cached list result if present.
// projectRoot, filter, and effectiveLimit must match the key used when the result was cached.
func GetListCache(projectRoot string, filter *ListFilter, effectiveLimit int) (*QueryResult, bool) {
	if projectRoot == emptyValue || filter == nil {
		return nil, false
	}
	key := listCacheKey(projectRoot, filter, effectiveLimit)
	var entry *listCacheEntry
	var ok bool
	var _err_83301007 = concurrency.RunInRLock(&globalListCache.mu, func() error {
		entry, ok = globalListCache.entries[key]
		return nil
	})
	if _err_83301007 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83301007).Log()
	}
	if listCacheMiss(ok, entry) {
		globalListCache.missesTotal.Add(1)
		return nil, false
	}
	// MCP used to bypass this cache entirely, so AgentX/hook object-list storms
	// re-Walked CAS on every call. Keep a short TTL so daemon lists stay fresh.
	// TRACK: BLI-CEF-STORAGE-INDEX-CACHE-001
	if IsMCPMode() && time.Since(entry.AddedAt) > mcpListCacheTTL {
		globalListCache.missesTotal.Add(1)
		return nil, false
	}
	globalListCache.hitsTotal.Add(1)
	return copyQueryResult(entry.Result), true
}

// SetListCache stores a copy of the list result in the cache.
func SetListCache(projectRoot string, filter *ListFilter, effectiveLimit int, result *QueryResult) {
	if projectRoot == emptyValue || filter == nil || result == nil {
		return
	}
	key := listCacheKey(projectRoot, filter, effectiveLimit)
	var _err_83301566 = concurrency.RunInLock(&globalListCache.mu, func() error {
		if globalListCache.entries == nil {
			globalListCache.entries = make(map[string]*listCacheEntry)
		}
		entries := globalListCache.entries
		if _, exists := entries[key]; !exists && len(entries) >= maxListCacheEntries {
			// Evict oldest by AddedAt so cache stays bounded (no unbounded growth).
			var oldestKey string
			var oldestTime time.Time
			first := true
			for k, e := range entries {
				if first || e.AddedAt.Before(oldestTime) {
					oldestKey = k
					oldestTime = e.AddedAt
					first = false
				}
			}
			if oldestKey != emptyValue {
				delete(entries, oldestKey)
			}
		}
		now := time.Now()
		entries[key] = &listCacheEntry{Result: copyQueryResult(result), AddedAt: now}
		return nil
	})
	if _err_83301566 !=

		// InvalidateListCache clears the entire list cache. Call when the object ID cache or
		// storage has been updated (e.g. object ID cache cleared/rebuilt, or after create/update/delete).
		nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83301566).Log()
	}
}

func InvalidateListCache() {
	globalListCache.invalidationsTotal.Add(1)
	var _err_83302693 = concurrency.RunInLock(&globalListCache.mu, func() error {
		globalListCache.entries = make(map[string]*listCacheEntry)
		return nil
	})
	if _err_83302693 !=

		// InvalidateListCacheForKind removes all list cache entries for the given kind.
		// Call when the object ID cache or storage for that kind has been updated (e.g. after
		// create/update/delete for that kind, or InvalidateObjectIDCacheKind(kind)).
		nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83302693).Log()
	}
}

func InvalidateListCacheForKind(kind string) {
	if kind == emptyValue {
		return
	}
	globalListCache.invalidationsTotal.Add(1)
	var _err_83303130 = concurrency.RunInLock(&globalListCache.mu, func() error {
		for key := range globalListCache.entries {

			parts := strings.SplitN(key, "|", 3)
			if len(parts) >= 2 && parts[1] == kind {
				delete(globalListCache.entries, key)
			}
		}
		return nil
	})
	if _err_83303130 !=

		// copyQueryResult returns a deep copy of the QueryResult for cache storage/return.
		nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83303130).Log()
	}
}

func copyQueryResult(r *QueryResult) *QueryResult {
	if r == nil {
		return nil
	}
	out := &QueryResult{
		Objects: make([]map[string]any, len(r.Objects)),
		Groups:  make(map[string][]map[string]any),
		Meta:    make(map[string]any),
	}
	for i, obj := range r.Objects {
		out.Objects[i] = copyMap(obj)
	}
	for k, list := range r.Groups {
		out.Groups[k] = make([]map[string]any, len(list))
		for i, obj := range list {
			out.Groups[k][i] = copyMap(obj)
		}
	}
	maps.Copy(out.Meta, r.Meta)
	return out
}

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	return out
}
