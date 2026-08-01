
package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	syscallutil "github.com/lanceman/zqk/pkg/syscallutil"
)

// ValidationState represents the validation state for an object
type ValidationState struct {
	ObjectID      string            `json:"object_id"`
	ObjectKind    string            `json:"object_kind"`
	FilePath      string            `json:"file_path"`
	LastValidated time.Time         `json:"last_validated"`
	Issues        []ValidationIssue `json:"issues"`
	Checksum      string            `json:"checksum"`     // File checksum to detect changes
	ValidatedBy   string            `json:"validated_by"` // Validation job ID
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// ValidationIssue represents a single validation issue
type ValidationIssue struct {
	Tier        int        `json:"tier"`                  // 1: blocking, 2: warning, 3: informational, 4: recommendation
	Category    string     `json:"category"`              // registration, lifecycle, policy, integrity, reference
	Message     string     `json:"message"`               // Issue description
	AutoFixable bool       `json:"auto_fixable"`          // Whether issue can be auto-fixed
	DetectedAt  time.Time  `json:"detected_at"`           // When issue was detected
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"` // When issue was resolved
}

// validationStateEntry is the compact persisted form: one entry per object, bucketed by kind.
// Path is relative to project root; when kind bucket has path_prefix, Path is suffix only.
type validationStateEntry struct {
	ID            string            `json:"id"`
	Path          string            `json:"path"`
	LastValidated time.Time         `json:"last_validated"`
	Checksum      string            `json:"checksum,omitempty"` // omit empty to avoid storing ""
	ValidatedBy   string            `json:"validated_by,omitempty"`
	Issues        []ValidationIssue `json:"issues,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// kindBucket is the per-kind bucket: optional path_prefix (so entries store path suffix only) and entries.
type kindBucket struct {
	PathPrefix string                 `json:"path_prefix,omitempty"`
	Entries    []validationStateEntry `json:"entries"`
}

// validationCacheFileV2 is the on-disk format: by_kind + metadata.
// Save writes kindBucket (path_prefix + entries with path suffix only). Load accepts kindBucket or legacy []validationStateEntry.
type validationCacheFileV2 struct {
	Version      string                `json:"version"`
	CodeChecksum string                `json:"code_checksum,omitempty"`
	Updated      time.Time             `json:"updated"`
	ByKind       map[string]kindBucket `json:"by_kind"`
}

// validationCacheExcludedKinds are high-volume or ephemeral kinds we do not persist
// in the validation cache so the file does not grow indefinitely (e.g. audit_event, metrics).
var validationCacheExcludedKinds = map[string]bool{
	objects.KindAuditEvent:            true,
	objects.KindBaseMetric:            true,
	objects.KindSchedulerHealthMetric: true,
	objects.KindCommandMetric:         true,
	objects.KindZqkSession:            true,
}

// ShouldCacheValidationState returns false for kinds that should not be stored in the
// persisted validation cache (avoids unbounded growth from high-volume/ephemeral objects).
func ShouldCacheValidationState(objectKind string) bool {
	return !validationCacheExcludedKinds[objectKind]
}

// ValidationStateCache manages cached validation states
type ValidationStateCache struct {
	cache       map[string]*ValidationState // Key: objectID, Value: validation state
	mu          sync.RWMutex
	cacheFile   string
	maxAge      time.Duration // Maximum age before revalidation
	projectRoot string
}

// NewValidationStateCache creates a new validation state cache
func NewValidationStateCache(projectRoot string, maxAge time.Duration) *ValidationStateCache {
	// Prefer brand-settings path alias ("cache") when the path cache is built, so cache location
	// is driven by zqk-settings.yaml. Fallback keeps the legacy .zqk/cache/validation_cache.json.
	cacheDir := paths.ResolvePathFromCacheOrConstant(projectRoot, PathAliasCache, filepath.Join(paths.ProjectDataDir, paths.CacheDir))
	cacheFile := filepath.Join(cacheDir, paths.ValidationCacheFile)
	return &ValidationStateCache{
		cache:       make(map[string]*ValidationState),
		cacheFile:   cacheFile,
		maxAge:      maxAge,
		projectRoot: projectRoot,
	}
}

// Load loads the validation cache from disk
func (c *ValidationStateCache) Load() error {
	if err := concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheLoad,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Create cache directory if it doesn't exist (I/O inside lock - should be refactored)
			if err := os.MkdirAll(filepath.Dir(c.cacheFile), paths.DirPerm755); err != nil {
				return errfmt.Newf(ErrMsgCreateCacheDir).Wrap(err)
			}

			// Check if cache file exists
			if _, err := os.Stat(c.cacheFile); os.IsNotExist(err) {
				// Cache doesn't exist yet, start with empty cache
				return nil
			}

			data, err := os.ReadFile(c.cacheFile)
			if err != nil {
				return errfmt.Newf(ErrMsgReadCacheFile).Wrap(err)
			}

			// Detect format: v2 has by_kind (object or array per kind), legacy has states
			var legacy struct {
				States       []*ValidationState         `json:"states"`
				Version      string                     `json:"version"`
				CodeChecksum string                     `json:"code_checksum,omitempty"`
				Updated      time.Time                  `json:"updated"`
				ByKindRaw    map[string]json.RawMessage `json:"by_kind"`
			}
			if err := json.Unmarshal(data, &legacy); err != nil {
				return nil
			}

			currentChecksum := computeValidationCodeChecksum(c.projectRoot)
			if legacy.CodeChecksum != emptyValue && legacy.CodeChecksum != currentChecksum {
				return nil
			}

			if len(legacy.ByKindRaw) > 0 {
				var capped bool
				for kind, raw := range legacy.ByKindRaw {
					if capped {
						break
					}
					if !ShouldCacheValidationState(kind) {
						continue
					}
					// Try new format: { "path_prefix": "...", "entries": [...] }
					var bucket kindBucket
					if err := json.Unmarshal(raw, &bucket); err == nil && len(bucket.Entries) > 0 {
						prefix := bucket.PathPrefix
						if prefix != emptyValue && !strings.HasSuffix(prefix, pathSeparator) {
							prefix += pathSeparator
						}
						for i := range bucket.Entries {
							if len(c.cache) >= MaxValidationCacheEntries {
								capped = true
								break
							}
							e := &bucket.Entries[i]
							if e.ID == emptyValue || len(e.ID) > MaxObjectIDLength {
								continue
							}
							relPath := e.Path
							if prefix != emptyValue {
								relPath = prefix + e.Path
							}
							fullPath := relPath
							if c.projectRoot != emptyValue && relPath != emptyValue && !filepath.IsAbs(relPath) {
								fullPath = filepath.Join(c.projectRoot, relPath)
							}
							c.cache[e.ID] = &ValidationState{
								ObjectID:      e.ID,
								ObjectKind:    kind,
								FilePath:      fullPath,
								LastValidated: e.LastValidated,
								Checksum:      e.Checksum,
								ValidatedBy:   e.ValidatedBy,
								Issues:        e.Issues,
								Metadata:      e.Metadata,
							}
						}
						if capped {
							break
						}
						continue
					}
					// Legacy: array of entries with full relative path
					var entries []validationStateEntry
					if err := json.Unmarshal(raw, &entries); err != nil {
						continue
					}
					for i := range entries {
						if len(c.cache) >= MaxValidationCacheEntries {
							capped = true
							break
						}
						e := &entries[i]
						if e.ID == emptyValue || len(e.ID) > MaxObjectIDLength {
							continue
						}
						fullPath := e.Path
						if c.projectRoot != emptyValue && e.Path != emptyValue && !filepath.IsAbs(e.Path) {
							fullPath = filepath.Join(c.projectRoot, e.Path)
						}
						c.cache[e.ID] = &ValidationState{
							ObjectID:      e.ID,
							ObjectKind:    kind,
							FilePath:      fullPath,
							LastValidated: e.LastValidated,
							Checksum:      e.Checksum,
							ValidatedBy:   e.ValidatedBy,
							Issues:        e.Issues,
							Metadata:      e.Metadata,
						}
					}
					if capped {
						break
					}
				}
			} else if len(legacy.States) > 0 {
				for _, state := range legacy.States {
					if len(c.cache) >= MaxValidationCacheEntries {
						break
					}
					if state != nil && state.ObjectID != emptyValue && len(state.ObjectID) <= MaxObjectIDLength && ShouldCacheValidationState(state.ObjectKind) {
						c.cache[state.ObjectID] = state
					}
				}
			}

			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// commonPathPrefix returns the longest common directory prefix of the given paths (all with forward slashes).
			// Returns "" if no common prefix or paths empty.
			Error(ErrMsgLockFailedLoad, err).Log()
	}
	return nil
}

func commonPathPrefix(paths []string) string {
	if len(paths) == 0 {
		return emptyValue
	}
	prefix := paths[0]
	// prefix is full path; we want directory prefix (including trailing /)
	if idx := strings.LastIndex(prefix, pathSeparator); idx >= 0 {
		prefix = prefix[:idx+1]
	} else {
		return emptyValue
	}
	for _, p := range paths[1:] {
		if idx := strings.LastIndex(p, pathSeparator); idx >= 0 {
			p = p[:idx+1]
		} else {
			return emptyValue
		}
		for i := 0; i < len(prefix) && i < len(p); i++ {
			if prefix[i] != p[i] {
				prefix = prefix[:i]
				if idx := strings.LastIndex(prefix, pathSeparator); idx >= 0 {
					prefix = prefix[:idx+1]
				} else {
					return emptyValue
				}
				break
			}
		}
		if len(p) < len(prefix) {
			prefix = p
		}
	}
	return prefix
}

// Save saves the validation cache to disk
// Uses file locking to ensure exclusive access during write (prevents corruption with multiple processes)
// FIXED: Don't hold lock during file I/O - acquire lock, copy data, release lock, then do I/O
func (c *ValidationStateCache) Save() error {
	// Acquire outer lock, build by_kind with path prefix + suffix-only paths, release lock
	var byKind map[string]kindBucket
	if err := concurrency.RunInRLockWithLogger(
		&c.mu,
		LockOpValidationCacheSaveCopy,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Group by kind and normalize paths to forward-slash relative
			kindPaths := make(map[string][]string)
			kindEntries := make(map[string][]validationStateEntry)
			for _, state := range c.cache {
				if len(state.ObjectID) > MaxObjectIDLength {
					continue
				}
				relPath := state.FilePath
				if c.projectRoot != emptyValue && state.FilePath != emptyValue {
					if r, err := filepath.Rel(c.projectRoot, state.FilePath); err == nil {
						relPath = filepath.ToSlash(r)
					}
				}
				kindEntries[state.ObjectKind] = append(kindEntries[state.ObjectKind], validationStateEntry{
					ID:            state.ObjectID,
					Path:          relPath,
					LastValidated: state.LastValidated,
					Checksum:      state.Checksum,
					ValidatedBy:   state.ValidatedBy,
					Issues:        state.Issues,
					Metadata:      state.Metadata,
				})
				kindPaths[state.ObjectKind] = append(kindPaths[state.ObjectKind], relPath)
			}
			byKind = make(map[string]kindBucket)
			for kind, entries := range kindEntries {
				paths := kindPaths[kind]
				prefix := commonPathPrefix(paths)
				suffixOnly := prefix != emptyValue
				for i := range entries {
					if suffixOnly {
						entries[i].Path = strings.TrimPrefix(entries[i].Path, prefix)
					}
				}
				byKind[kind] = kindBucket{
					PathPrefix: prefix,
					Entries:    entries,
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Create cache directory if it doesn't exist (no lock held)
			Error(ErrMsgLockFailedSave, err).Log()
	}

	if err := os.MkdirAll(filepath.Dir(c.cacheFile), paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateCacheDir).Wrap(err)
	}

	codeChecksum := computeValidationCodeChecksum(c.projectRoot)
	cacheData := validationCacheFileV2{
		Version:      ValidationCacheVersion,
		CodeChecksum: codeChecksum,
		Updated:      time.Now(),
		ByKind:       byKind,
	}

	data, err := json.Marshal(cacheData)
	if err != nil {
		return errfmt.Newf(ErrMsgMarshalCache).Wrap(err)
	}

	// Use file locking to prevent concurrent writes from multiple processes
	lockFile := c.cacheFile + paths.LockFileSuffix

	// Check for stale locks (locks held by dead/hung processes)
	// If lock file exists and is older than 30 seconds, it's likely stale
	// Note: On Unix, locks are automatically released when a process dies,
	// but if a process hangs, the lock remains. We detect stale locks by checking
	// the lock file's modification time.
	staleLockTimeout := 30 * time.Second
	if stat, err := os.Stat(lockFile); err == nil {
		if time.Since(stat.ModTime()) > staleLockTimeout {
			// Lock file is old - likely from a hung process
			// Try to remove it (safe if process is dead, harmless if process is alive)
			// The actual lock is held by the file descriptor, not the file itself,
			// so removing the file won't break an active lock, but it will allow
			// us to create a new one if the process is truly dead.
			if err := os.Remove(lockFile); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveStaleLock, err).Log()
			}
		}
	}

	file, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf(ErrMsgOpenLockFile).Wrap(err)
	}
	defer file.Close()

	// Acquire exclusive lock (non-blocking)
	// If lock is held by another process, return error (caller can retry)
	// Note: If a process dies while holding a lock, the OS automatically releases it.
	// If a process hangs, the lock remains until the process dies or releases it.
	err = syscallutil.FileFlock(file, syscallutil.LockExNb)
	if err != nil {
		// Lock is held by another process - check if it's stale
		if stat, statErr := os.Stat(lockFile); statErr == nil {
			if time.Since(stat.ModTime()) > staleLockTimeout {
				// Lock file is stale - process may be hung
				// Try to break the lock by closing and removing the file
				// This is safe because:
				// 1. If process is dead, lock is already released by OS
				// 2. If process is alive, removing the file won't affect its lock (lock is on FD)
				// 3. We can then try again
				file.Close()
				if err := os.Remove(lockFile); err != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

						// Retry once after removing stale lock file
						Error(ErrMsgRemoveStaleLock, err).Log()
				}

				file2, err2 := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, paths.FilePerm644)
				if err2 == nil {
					err2 = syscallutil.FileFlock(file2, syscallutil.LockExNb)
					if err2 == nil {
						// Successfully acquired lock after breaking stale lock
						file = file2
						// Update lock file modification time to current time
						now := time.Now()
						if err := os.Chtimes(lockFile, now, now); err != nil {
							logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgUpdateLockTimestamp, err).Log()
						}
					} else {
						file2.Close()
						return errfmt.Errorf(ErrMsgStaleLockCleanup)
					}
				} else {
					return errfmt.Errorf(ErrMsgRecreateLockFailed)
				}
			} else {
				// Lock is recent - process is likely still active
				return errfmt.Errorf(ErrMsgCacheLocked)
			}
		} else {
			// Can't stat lock file - return error
			return errfmt.Errorf(ErrMsgCacheLocked)
		}
	} else {
		// Successfully acquired lock - update modification time
		now := time.Now()
		if err := os.Chtimes(lockFile, now, now); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgUpdateLockTimestamp, err).Log()
		}
	}
	defer func() {
		if err := syscallutil.FileFlock(file, syscallutil.LockUn); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Do not overwrite a populated cache with an empty one (e.g. process with no validation
				// state wins the lock and truncates the file that another process had filled).
				Error(ErrMsgReleaseLock, err).Log()
		}
	}()

	if len(byKind) == 0 {
		if stat, statErr := os.Stat(c.cacheFile); statErr == nil && stat.Size() > 0 {
			existingData, readErr := os.ReadFile(c.cacheFile)
			if readErr == nil {
				var existing struct {
					ByKind map[string]json.RawMessage `json:"by_kind"`
				}
				if json.Unmarshal(existingData, &existing) == nil && len(existing.ByKind) > 0 {
					return nil
				}
			}
		}
	}

	// Write to temporary file first, then rename (atomic write)
	tmpFile := c.cacheFile + paths.TmpFileSuffix
	if err := os.WriteFile(tmpFile, data, paths.FilePerm600); err != nil {
		return errfmt.Newf(ErrMsgWriteCacheFile).Wrap(err)
	}

	if err := os.Rename(tmpFile, c.cacheFile); err != nil {
		return errfmt.Newf(ErrMsgRenameCacheFile).Wrap(err)
	}

	return nil
}

// Get retrieves validation state for an object
func (c *ValidationStateCache) Get(objectID string) (*ValidationState, bool) {
	var state *ValidationState
	var exists bool
	if err := concurrency.RunInRLockWithLogger(
		&c.mu,
		LockOpValidationCacheGet,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			state, ok = c.cache[objectID]
			exists = ok
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGet, err).Log()
	}

	if !exists {
		return nil, false
	}

	// Check if state is stale (need maxAge - copy it out of lock)
	var maxAge time.Duration
	if err := concurrency.RunInRLockWithLogger(
		&c.mu,
		LockOpValidationCacheGetMaxAge,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			maxAge = c.maxAge
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGet, err).Log()
	}

	if maxAge > 0 && time.Since(state.LastValidated) > maxAge {
		return nil, false
	}

	return state, true
}

// GetAll retrieves all validation states from cache
func (c *ValidationStateCache) GetAll() []*ValidationState {
	var states []*ValidationState
	var maxAge time.Duration
	if err := concurrency.RunInRLockWithLogger(
		&c.mu,
		LockOpValidationCacheGetAll,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			maxAge = c.maxAge
			states = make([]*ValidationState, 0, len(c.cache))
			for _, state := range c.cache {
				// Only return non-stale states (maxAge = 0 means no expiration)
				if maxAge == 0 || time.Since(state.LastValidated) <= maxAge {
					states = append(states, state)
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGetAll, err).Log()
	}
	return states
}

// Set stores validation state for an object. Entries with object ID longer than
// MaxObjectIDLength are not stored so the cache never persists invalid IDs.
// If at MaxValidationCacheEntries, evicts the entry with oldest LastValidated before adding.
func (c *ValidationStateCache) Set(state *ValidationState) {
	if state == nil || state.ObjectID == emptyValue || len(state.ObjectID) > MaxObjectIDLength {
		return
	}
	if err := concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheSet,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			_, updating := c.cache[state.ObjectID]
			if !updating && len(c.cache) >= MaxValidationCacheEntries {
				// Evict oldest by LastValidated to bound memory
				var oldestID string
				var oldestTime time.Time
				first := true
				for id, s := range c.cache {
					if first || s.LastValidated.Before(oldestTime) {
						oldestID = id
						oldestTime = s.LastValidated
						first = false
					}
				}
				if oldestID != emptyValue {
					delete(c.cache, oldestID)
				}
			}
			c.cache[state.ObjectID] = state
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Invalidate marks validation state as invalid (forces revalidation)
			Error(ErrMsgLockFailedSet, err).Log()
	}
}

func (c *ValidationStateCache) Invalidate(objectID string) {
	if err := concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheInvalidate,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(c.cache, objectID)
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// InvalidateByKind invalidates all states for a specific kind
			Error(ErrMsgLockFailedInvalidate, err).Log()
	}
}

func (c *ValidationStateCache) InvalidateByKind(kind string) {
	if err := concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheInvalidateKind,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for id, state := range c.cache {
				if state.ObjectKind == kind {
					delete(c.cache, id)
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// InvalidateByCategory invalidates all states with issues in a specific category
			Error(ErrMsgLockFailedInvalidate, err).Log()
	}
}

func (c *ValidationStateCache) InvalidateByCategory(category string) int {
	var count int
	if err := concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheInvalidateCat,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			count = 0
			for id, state := range c.cache {
				for _, issue := range state.Issues {
					if issue.Category == category {
						delete(c.cache, id)
						count++
						break // Only count each object once
					}
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedInvalidate, err).Log()
	}
	return count
}

// InvalidateByPattern invalidates states matching a pattern
func (c *ValidationStateCache) InvalidateByPattern(pattern string) {
	if err := concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheInvalidatePat,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Simple prefix matching for now
			for id := range c.cache {
				if strings.HasPrefix(id, pattern) {
					delete(c.cache, id)
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Clear clears all validation states from cache
			Error(ErrMsgLockFailedInvalidatePat, err).Log()
	}
}

func (c *ValidationStateCache) Clear() error {
	var cacheFile, lockFile string
	if err := concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheClear,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Clear in-memory cache
			c.cache = make(map[string]*ValidationState)
			cacheFile = c.cacheFile
			lockFile = c.cacheFile + paths.LockFileSuffix
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Remove cache file if it exists (NO LOCK HELD during I/O)
			Error(ErrMsgLockFailedClear, err).Log()
	}

	if _, err := os.Stat(cacheFile); err == nil {
		if err := os.Remove(cacheFile); err != nil {
			return errfmt.Newf(ErrMsgRemoveCacheFile).Wrap(err)
		}
	}

	// Also remove lock file if it exists
	if _, err := os.Stat(lockFile); err == nil {
		if err := os.Remove(lockFile); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveLockFile, err).Log()
		}
	}

	return nil
}

// InvalidateAll clears all cached validation states, forcing re-validation
// This is useful when hash registries are updated and cached hash mismatches
// need to be re-evaluated
func (c *ValidationStateCache) InvalidateAll() error {
	return c.Clear()
}

// GetStale returns all stale validation states (older than maxAge)
func (c *ValidationStateCache) GetStale() []*ValidationState {
	var stale []*ValidationState
	var maxAge time.Duration
	if err := concurrency.RunInRLockWithLogger(
		&c.mu,
		LockOpValidationCacheGetStale,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			maxAge = c.maxAge
			now := time.Now()
			for _, state := range c.cache {
				if maxAge > 0 && now.Sub(state.LastValidated) > maxAge {
					stale = append(stale, state)
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGetStale, err).Log()
	}
	return stale
}

// GetByTier returns all validation states with issues of a specific tier
func (c *ValidationStateCache) GetByTier(tier int) []*ValidationState {
	var results []*ValidationState
	if err := concurrency.RunInRLockWithLogger(
		&c.mu,
		LockOpValidationCacheGetByTier,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, state := range c.cache {
				for _, issue := range state.Issues {
					if issue.Tier == tier && issue.ResolvedAt == nil {
						results = append(results, state)
						break
					}
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGetByTier, err).Log()
	}
	return results
}

// Count returns statistics about the cache
func (c *ValidationStateCache) Count() (total, stale, withIssues int) {
	var totalCount, staleCount, withIssuesCount int
	var maxAge time.Duration
	if err := concurrency.RunInRLockWithLogger(
		&c.mu,
		LockOpValidationCacheCount,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			maxAge = c.maxAge
			now := time.Now()
			for _, state := range c.cache {
				totalCount++
				if maxAge > 0 && now.Sub(state.LastValidated) > maxAge {
					staleCount++
				}
				if len(state.Issues) > 0 {
					hasUnresolved := false
					for _, issue := range state.Issues {
						if issue.ResolvedAt == nil {
							hasUnresolved = true
							break
						}
					}
					if hasUnresolved {
						withIssuesCount++
					}
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedCount, err).Log()
	}
	return totalCount, staleCount, withIssuesCount
}

// computeValidationCodeChecksum computes a checksum of validation code files
// that affect how validation results are generated (especially auto-fixable detection).
// Uses ValidationCodeChecksumFiles so the list is maintained in one place.
func computeValidationCodeChecksum(projectRoot string) string {
	hasher := sha256.New()
	anyFileFound := false

	for _, relPath := range ValidationCodeChecksumFiles {
		fullPath := filepath.Join(projectRoot, relPath)

		// Try to read file (may not exist in all environments)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			// File doesn't exist or can't be read - skip it
			// This is okay for environments where source code isn't available
			continue
		}

		anyFileFound = true
		// Include both path and content in checksum
		hasher.Write([]byte(relPath))
		hasher.Write([]byte(lineSeparator))
		hasher.Write(data)
		hasher.Write([]byte(lineSeparator))
	}

	if !anyFileFound {
		return NoSourceCodeAvailableChecksum
	}

	return hex.EncodeToString(hasher.Sum(nil))
}
