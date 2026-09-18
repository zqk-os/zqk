// Package paths: path alias cache and resolution by scheme (prefix:, abs:, web:).
// Paths are resolved at runtime from the cache built at pre-warm so any path in the project
// uses aliases and scheme prefixes instead of hardcoded paths. See PATH_ALIAS_RESOLUTION.md.
//
// The cache uses a snapshot-and-swap design: each project root has an immutable snapshot (alias map + builtAt).
// Builds can run in the background and swap in the new snapshot so readers are never blocked by a refresh.
// Use ReplacePathCache for a full replace (e.g. async refresh); BuildPathAliasCache to merge; AddPathAlias/RemovePathAlias for incremental updates.
package paths

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/errfmt"
)

// pathCacheSnapshot is an immutable snapshot of the alias map for one project root.
type pathCacheSnapshot struct {
	aliases map[string]string
	builtAt time.Time
}

var (
	pathAliasMu    sync.RWMutex
	pathAliasCache = make(map[string]*pathCacheSnapshot) // projectRoot -> immutable snapshot
)

// ErrPathAliasNotInCache is returned when a prefix: alias is not in the path cache (e.g. cache not built or stale).
// Callers should trigger path-cache build/refresh or return a clear error to the user instead of resolving silently.
var ErrPathAliasNotInCache = errfmt.Errorf("path alias not in cache: run path-cache check or ensure pre-warm has run")

const emptyPathValue = ""

// pathAliasMetrics is the path-cache alias for the project metrics subtree under .zqk.
// String matches objects.FieldKeyMetrics, but we do not import pkg/objects here (cycle with context/paths).
const pathAliasMetrics = "metrics"

// Path-cache aliases for pkg/datacell runtime organism files (feature flags, CLI hook profile, tray).
// See docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md and [ResolvePathFromCacheOrConstant].
const (
	PathAliasDatacellFeatureFlags    = "datacell_feature_flags"
	PathAliasDatacellCLIHookProfile  = "datacell_cli_hook_profile"
	PathAliasDatacellTrayYAML        = "datacell_tray_yaml"
	PathAliasDatacellRuntimeManifest = "datacell_runtime_manifest"
	// Agent chat channel pilot (lite config + JSONL events). See pkg/datacell/agent_chat_channel.go.
	PathAliasDatacellAgentChatChannelConfig = "datacell_agent_chat_channel_config"
	PathAliasDatacellAgentChatChannelEvents = "datacell_agent_chat_channel_events"
	// Agent idleness tracking file (lite file pattern).
	PathAliasDatacellAgentIdleStore = "datacell_agent_idle_store"
	// Steward enqueue queue JSONL (.zqk/logs/datacell/steward_enqueue.jsonl by default). See pkg/datacell.AppendStewardEnqueueRecord.
	PathAliasDatacellStewardEnqueue = "datacell_steward_enqueue"

	// MCP / mesh runtime layout (daemon PID/logs, IDE install, peer seats). Prefer these over hardcoded ".zqk/...".
	PathAliasMCP              = "mcp"
	PathAliasMCPConfig        = "mcp_config"
	PathAliasWorkshopBin      = "workshop_bin"
	PathAliasRepoBin          = "repo_bin"
	PathAliasMeshState        = "mesh_state"
	PathAliasMeshPeerSeats    = "mesh_peer_seats"
	PathAliasMeshPeerAckAwait = "mesh_peer_ack_awaits"
)

// copySnapshot returns a mutable copy of the current alias map for projectRoot. Must be called from within RunInRLock(&pathAliasMu, ...).
func copySnapshot(projectRoot string) map[string]string {
	snap := pathAliasCache[projectRoot]
	if snap == nil || len(snap.aliases) == 0 {
		return make(map[string]string)
	}
	out := make(map[string]string, len(snap.aliases)+8)
	maps.Copy(out, snap.aliases)
	return out
}

// replacePathCache replaces the cache for projectRoot with the given alias map (atomic swap).
// Used for full rebuilds (e.g. background refresh) and after merging in BuildPathAliasCache.
func replacePathCache(projectRoot string, aliases map[string]string) {
	if projectRoot == emptyPathValue {
		return
	}
	_ = concurrency.RunInLock(&pathAliasMu, func() error {
		cleaned := make(map[string]string, len(aliases))
		for k, v := range aliases {
			cleaned[k] = filepath.Clean(v)
		}
		pathAliasCache[projectRoot] = &pathCacheSnapshot{aliases: cleaned, builtAt: time.Now()}
		return nil
	})
}

// ReplacePathCache replaces the path alias cache for projectRoot with the given alias map (atomic swap).
// Use for background refresh: build the full alias map off the lock, then call ReplacePathCache so readers see the new cache without blocking on build.
func ReplacePathCache(projectRoot string, aliases map[string]string) {
	replacePathCache(projectRoot, aliases)
}

// BuildPathAliasCache merges aliases into the path cache for projectRoot (copy current snapshot, merge, swap).
// Call from scheduler cache pre-warm (Tier 0), CLI path-cache, or when adding doc_entry paths.
func BuildPathAliasCache(projectRoot string, aliases map[string]string) {
	if projectRoot == emptyPathValue || len(aliases) == 0 {
		return
	}
	var merged map[string]string
	_ = concurrency.RunInRLock(&pathAliasMu, func() error {
		merged = copySnapshot(projectRoot)
		return nil
	})
	for k, v := range aliases {
		merged[k] = filepath.Clean(v)
	}
	replacePathCache(projectRoot, merged)
}

// AddPathAlias adds or updates a single alias for projectRoot (incremental update; copy current, add one, swap).
// Use when a new path is added (e.g. new doc_entry or new folder). No-op if projectRoot or alias is empty.
func AddPathAlias(projectRoot, alias, relPath string) {
	if projectRoot == emptyPathValue || alias == emptyPathValue {
		return
	}
	var merged map[string]string
	_ = concurrency.RunInRLock(&pathAliasMu, func() error {
		merged = copySnapshot(projectRoot)
		return nil
	})
	merged[alias] = filepath.Clean(relPath)
	replacePathCache(projectRoot, merged)
}

// RemovePathAlias removes a single alias for projectRoot (incremental update; copy current without alias, swap).
// Use when a path is removed (e.g. doc_entry deleted or folder removed). No-op if projectRoot or alias is empty.
func RemovePathAlias(projectRoot, alias string) {
	if projectRoot == emptyPathValue || alias == emptyPathValue {
		return
	}
	var merged map[string]string
	_ = concurrency.RunInRLock(&pathAliasMu, func() error {
		merged = copySnapshot(projectRoot)
		return nil
	})
	delete(merged, alias)
	replacePathCache(projectRoot, merged)
}

// IsPathCacheBuilt returns true if the path alias cache has been built for projectRoot.
func IsPathCacheBuilt(projectRoot string) bool {
	if projectRoot == emptyPathValue {
		return false
	}
	var ok bool
	_ = concurrency.RunInRLock(&pathAliasMu, func() error {
		snap := pathAliasCache[projectRoot]
		ok = snap != nil && len(snap.aliases) > 0
		return nil
	})
	return ok
}

// IsPathCacheStale returns true if the cache was built before any of the given dirs were last modified.
// checkDirs are paths under projectRoot (or projectRoot itself). If any has mtime > cache build time, cache is stale.
// Pass nil or empty to skip mtime check (only "never built" would be stale via IsPathCacheBuilt).
func IsPathCacheStale(projectRoot string, checkDirs []string) bool {
	if projectRoot == emptyPathValue {
		return true
	}
	var builtAt time.Time
	var snap *pathCacheSnapshot
	_ = concurrency.RunInRLock(&pathAliasMu, func() error {
		snap = pathAliasCache[projectRoot]
		if snap != nil {
			builtAt = snap.builtAt
		}
		return nil
	})
	if snap == nil {
		return true
	}
	for _, rel := range checkDirs {
		p := filepath.Join(projectRoot, filepath.Clean(rel))
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.ModTime().After(builtAt) {
			return true
		}
	}
	return false
}

// DefaultStalenessCheckDirs returns the default list of paths (relative to project root) to check for cache staleness.
// Configurable later via config/zqk.yaml; for now .zqk and .zqk/config are critical.
func DefaultStalenessCheckDirs() []string {
	return []string{ProjectDataDir, filepath.Join(ProjectDataDir, ConfigDir)}
}

// ResolvePath resolves a path reference using the scheme prefix and alias cache.
// pathRef may be: prefix:<alias>, abs:<uri>, web:<url>; no scheme is treated as prefix.
// For prefix: when alias is not in cache, returns filepath.Join(projectRoot, alias) as fallback.
// Prefer ResolvePathStrict in new code so cache miss returns an error instead of silent resolution.
func ResolvePath(projectRoot, pathRef string) string {
	if pathRef == emptyValue {
		return ""
	}
	if projectRoot == emptyValue && !strings.HasPrefix(pathRef, PathSchemeAbs) && !strings.HasPrefix(pathRef, PathSchemeWeb) {
		return pathRef
	}
	switch {
	case strings.HasPrefix(pathRef, PathSchemeWeb):
		return strings.TrimPrefix(pathRef, PathSchemeWeb)
	case strings.HasPrefix(pathRef, PathSchemeAbs):
		return strings.TrimPrefix(pathRef, PathSchemeAbs)
	case strings.HasPrefix(pathRef, PathSchemePrefix):
		alias := strings.TrimPrefix(pathRef, PathSchemePrefix)
		return resolvePrefix(projectRoot, alias)
	default:
		return resolvePrefix(projectRoot, pathRef)
	}
}

func resolvePrefix(projectRoot, alias string) string {
	s, _ := resolvePrefixStrict(projectRoot, alias)
	if s != emptyValue {
		return s
	}
	return filepath.Join(projectRoot, filepath.Clean(alias))
}

// ResolvePathStrict is like ResolvePath but returns ErrPathAliasNotInCache when a prefix: alias
// is not in the cache, so callers can fail fast or trigger cache build instead of resolving silently.
func ResolvePathStrict(projectRoot, pathRef string) (string, error) {
	if pathRef == emptyValue {
		return "", nil
	}
	if projectRoot == emptyValue && !strings.HasPrefix(pathRef, PathSchemeAbs) && !strings.HasPrefix(pathRef, PathSchemeWeb) {
		return pathRef, nil
	}
	switch {
	case strings.HasPrefix(pathRef, PathSchemeWeb):
		return strings.TrimPrefix(pathRef, PathSchemeWeb), nil
	case strings.HasPrefix(pathRef, PathSchemeAbs):
		return strings.TrimPrefix(pathRef, PathSchemeAbs), nil
	case strings.HasPrefix(pathRef, PathSchemePrefix):
		alias := strings.TrimPrefix(pathRef, PathSchemePrefix)
		return resolvePrefixStrict(projectRoot, alias)
	default:
		return resolvePrefixStrict(projectRoot, pathRef)
	}
}

func resolvePrefixStrict(projectRoot, alias string) (string, error) {
	var rel string
	_ = concurrency.RunInRLock(&pathAliasMu, func() error {
		snap := pathAliasCache[projectRoot]
		if snap != nil && snap.aliases != nil {
			rel = snap.aliases[alias]
			if rel == emptyValue {
				// e.g. process_internal/spec_index.json -> alias process_internal + suffix
				if i := strings.Index(alias, "/"); i > 0 {
					prefix := alias[:i]
					suffix := alias[i+1:]
					if base := snap.aliases[prefix]; base != emptyValue && suffix != emptyValue {
						rel = filepath.Join(base, suffix)
					}
				}
			}
		}
		return nil
	})
	if rel != emptyValue {
		return filepath.Join(projectRoot, rel), nil
	}
	return "", ErrPathAliasNotInCache
}

// PathRefFromRelPath returns a path-cache reference for a relative path (prefix:+relPath).
// Use when storing paths in objects (e.g. doc_entry.path) so resolution goes through the cache.
func PathRefFromRelPath(relPath string) string {
	if relPath == emptyValue {
		return ""
	}
	return PathSchemePrefix + filepath.ToSlash(filepath.Clean(relPath))
}

// NormalizeDocEntryPathForKey returns the canonical form for use as a map key (e.g. existing[path]=id).
// Strips prefix: so that "prefix:docs/foo.md" and "docs/foo.md" both yield "docs/foo.md".
func NormalizeDocEntryPathForKey(pathRef string) string {
	if pathRef == emptyValue {
		return ""
	}
	s := strings.TrimPrefix(pathRef, PathSchemePrefix)
	return filepath.ToSlash(filepath.Clean(s))
}

// ResolveDocEntryPath resolves a doc_entry path attribute to an absolute path.
// pathRef may be a path-cache ref (prefix:...), abs:, web:, or a legacy bare relative path.
// Returns the absolute path for prefix: (via cache), or passthrough for abs:/web: (caller uses as URI/URL),
// or filepath.Join(projectRoot, pathRef) for legacy relative. For prefix: cache miss returns ErrPathAliasNotInCache.
func ResolveDocEntryPath(projectRoot, pathRef string) (string, error) {
	if pathRef == emptyValue {
		return "", nil
	}
	switch {
	case strings.HasPrefix(pathRef, PathSchemeWeb), strings.HasPrefix(pathRef, PathSchemeAbs):
		return strings.TrimPrefix(strings.TrimPrefix(pathRef, PathSchemeWeb), PathSchemeAbs), nil
	case strings.HasPrefix(pathRef, PathSchemePrefix):
		return ResolvePathStrict(projectRoot, pathRef)
	default:
		return filepath.Join(projectRoot, filepath.Clean(pathRef)), nil
	}
}

// GetPathAlias returns the cached relative path for an alias, or "" if not in cache.
// Useful when callers need only the relative path. ResolvePath(projectRoot, "prefix:"+alias)
// is equivalent to filepath.Join(projectRoot, GetPathAlias(projectRoot, alias)) when cached.
func GetPathAlias(projectRoot, alias string) string {
	if projectRoot == emptyValue || alias == emptyValue {
		return ""
	}
	var rel string
	_ = concurrency.RunInRLock(&pathAliasMu, func() error {
		snap := pathAliasCache[projectRoot]
		if snap != nil && snap.aliases != nil {
			rel = snap.aliases[alias]
		}
		return nil
	})
	return rel
}

// ResolvePathFromCacheOrConstant returns the absolute path for alias when the path cache has been built
// (so paths come from brand settings), otherwise filepath.Join(projectRoot, fallbackRel).
// Use this for path construction (e.g. cache dir, config dir) so moving folders only requires editing
// config/zqk.yaml; fallbackRel should be the constant-based relative path (e.g. filepath.Join(ProjectDataDir, CacheDir)).
func ResolvePathFromCacheOrConstant(projectRoot, alias, fallbackRel string) string {
	if projectRoot == emptyValue {
		return ""
	}
	if rel := GetPathAlias(projectRoot, alias); rel != emptyValue {
		return filepath.Join(projectRoot, rel)
	}
	return filepath.Join(projectRoot, filepath.Clean(fallbackRel))
}

// DefaultPathAliases returns the default alias -> relative path map for the project structure.
// Used by the path alias cache build at pre-warm. Callers may merge additional aliases (e.g. stream kinds).
func DefaultPathAliases() map[string]string {
	return map[string]string{
		"docs":                                  DocsDir,
		"process":                               ProcessDir,
		"architecture":                          ProcessArchitectureDir,
		"onboarding":                            OnboardingDir,
		"pkg":                                   PkgDir,
		"cmd":                                   CmdDir,
		"scripts":                               ScriptsDir,
		"tools":                                 ToolsDir,
		"internal":                              InternalDir,
		"zqk":                                   ProjectDataDir,
		".zqk":                                  ProjectDataDir,
		"streams":                               filepath.Join(ProjectDataDir, StreamsDir),
		"config":                                filepath.Join(ProjectDataDir, ConfigDir),
		"cache":                                 filepath.Join(ProjectDataDir, CacheDir),
		"logs":                                  filepath.Join(ProjectDataDir, LogsDir),
		"state":                                 filepath.Join(ProjectDataDir, StateDir),
		pathAliasMetrics:                        filepath.Join(ProjectDataDir, MetricsDir),
		"scheduler":                             filepath.Join(ProjectDataDir, SchedulerDir),
		"pre_commit":                            filepath.Join(ProjectDataDir, PreCommitDir),
		"drafts":                                filepath.Join(ProjectDataDir, DraftsDir),
		"object_drafts":                         filepath.Join(ProjectDataDir, ObjectDraftsDir),
		PathAliasMCP:                            filepath.Join(ProjectDataDir, MCPDir),
		PathAliasMCPConfig:                      filepath.Join(ProjectDataDir, MCPDir, MCPConfigFile),
		PathAliasWorkshopBin:                    filepath.Join(ProjectDataDir, WorkshopBinDir),
		PathAliasRepoBin:                        RepoBinDir,
		PathAliasMeshState:                      filepath.Join(ProjectDataDir, StateDir, MeshStateSubdir),
		PathAliasMeshPeerSeats:                  filepath.Join(ProjectDataDir, StateDir, MeshStateSubdir, PeerSeatsFile),
		PathAliasMeshPeerAckAwait:               filepath.Join(ProjectDataDir, StateDir, MeshStateSubdir, PeerAckAwaitsFile),
		PathAliasDatacellFeatureFlags:           filepath.Join(ProjectDataDir, ConfigDir, FeatureFlagsFile),
		PathAliasDatacellCLIHookProfile:         filepath.Join(ProjectDataDir, ConfigDir, CLIHookProfileFile),
		PathAliasDatacellTrayYAML:               filepath.Join(ProjectDataDir, TrayYAMLFile),
		PathAliasDatacellRuntimeManifest:        filepath.Join(ProjectDataDir, ConfigDir, DataCellRuntimeManifestFile),
		PathAliasDatacellAgentChatChannelConfig: filepath.Join(ProjectDataDir, ConfigDir, AgentChatChannelConfigFile),
		PathAliasDatacellAgentChatChannelEvents: filepath.Join(ProjectDataDir, LogsDir, IDEHooksLogsSubdir, AgentChatChannelEventsFile),
		PathAliasDatacellAgentIdleStore:         filepath.Join(ProjectDataDir, ConfigDir, AgentIdleStoreFile),
		PathAliasDatacellStewardEnqueue:         filepath.Join(ProjectDataDir, LogsDir, DataCellLogsSubdir, StewardEnqueueJSONLFile),
		"process_internal":                      ProcessInternalDir,
		"process_policies":                      ProcessPoliciesDir,
		"process_goals":                         ProcessGoalsDir,
		"agent_skills":                          ProcessAgentSkillsDir,
	}
}

// MustNotDestroyProjectRoot validates that the given target path is not the project root
// itself or its core data directories. It returns an error if the path is unsafe to remove.
// This prevents catastrophic data loss from unguarded os.RemoveAll operations.
func MustNotDestroyProjectRoot(projectRoot, targetPath string) error {
	if projectRoot == "" || targetPath == "" {
		return errfmt.Errorf("PROJECT_ROOT_WIPE_HAZARD: project root or target path cannot be empty")
	}
	projClean := filepath.Clean(projectRoot)
	targetClean := filepath.Clean(targetPath)

	if projClean == targetClean {
		return errfmt.Errorf("PROJECT_ROOT_WIPE_HAZARD: attempt to destroy project root %s", projClean)
	}

	if targetClean == filepath.Join(projClean, ProjectDataDir) || targetClean == filepath.Join(projClean, ".git") {
		return errfmt.Errorf("PROJECT_ROOT_WIPE_HAZARD: attempt to destroy core project dir %s", targetClean)
	}

	return nil
}
