package validation

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	syscallutil "github.com/lanceman/zqk/pkg/utils/syscallutil"
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
	// loadedUpdated is the on-disk `updated` timestamp from the last Load.
	// Save refuses to overwrite a file whose Updated is newer (another process
	// already swapped in a fresher generation). TRACK: BLI-REDACTED
	loadedUpdated time.Time
	// replaceDiskOnNextSave is set by Clear so this instance's next Save wins
	// even if a peer restored the old file between unlink and persist.
	replaceDiskOnNextSave bool
	// suppressLoadUntilSave blocks Start()/Load from refilling memory from a
	// raced restore of the file Clear just deleted.
	suppressLoadUntilSave bool
	// saveMu serializes Save() from the same process so overlapping flushers coalesce.
	saveMu sync.Mutex
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
	var discardStaleFile bool
	if err := concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheLoad,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if c.suppressLoadUntilSave {
				return nil
			}
			// Create cache directory if it doesn't exist (I/O inside lock - should be refactored)
			if err := fileutil.MkdirAll(filepath.Dir(c.cacheFile), paths.DirPerm755); err != nil {
				return errfmt.Newf(ErrMsgCreateCacheDir).Wrap(err)
			}

			// Check if cache file exists
			if _, err := fileutil.Stat(c.cacheFile); fileutil.IsNotExist(err) {
				// Cache doesn't exist yet, start with empty cache
				return nil
			}

			data, err := fileutil.ReadFile(c.cacheFile)
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
			// Empty checksum is unknown generation: drop it. Matching
			// NoSourceCodeAvailableChecksum is kept only when we still cannot
			// fingerprint (no executable, no source). TRACK: PRI-CEF-R26-LIFECYCLE-EXAM-001
			if currentChecksum != NoSourceCodeAvailableChecksum && legacy.CodeChecksum != currentChecksum {
				// Do not leave a poison on-disk file: empty Save() later can hit the
				// empty-overwrite guard and preserve stale Tier-1 hits across processes.
				// TRACK: BLI-REDACTED
				c.cache = make(map[string]*ValidationState)
				discardStaleFile = true
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

			if !legacy.Updated.IsZero() {
				c.loadedUpdated = legacy.Updated
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Error(ErrMsgLockFailedLoad, err).Log()
		// Preserve historical soft-fail: empty in-memory cache, caller continues.
	}
	if discardStaleFile {
		if remErr := fileutil.Remove(c.cacheFile); remErr != nil && !fileutil.IsNotExist(remErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Error(ErrMsgRemoveCacheFile, remErr).Log()
		}
		lockFile := c.cacheFile + paths.LockFileSuffix
		if remErr := fileutil.Remove(lockFile); remErr != nil && !fileutil.IsNotExist(remErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Error(ErrMsgRemoveLockFile, remErr).Log()
		}
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
	c.saveMu.Lock()
	defer c.saveMu.Unlock()

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
		// Do not fall through and write a nil/empty by_kind — that truncates a
		// populated on-disk cache and breaks the system-check warm path.
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Error(ErrMsgLockFailedSave, err).Log()
		return errfmt.Errorf(ErrMsgLockFailedSave, err)
	}
	if byKind == nil {
		byKind = make(map[string]kindBucket)
	}

	if err := fileutil.MkdirAll(filepath.Dir(c.cacheFile), paths.DirPerm755); err != nil {
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

	// Write to temporary file first, outside the lock, to minimize lock duration
	tmpFileName := fmt.Sprintf("%s.%d", c.cacheFile+paths.TmpFileSuffix, time.Now().UnixNano())
	if err := fileutil.WriteFile(tmpFileName, data, paths.FilePerm600); err != nil {
		return errfmt.Newf(ErrMsgWriteCacheFile).Wrap(err)
	}
	defer func() {
		_ = fileutil.Remove(tmpFileName) // clean up if unused; ignore error if renamed
	}()

	// Use file locking to prevent concurrent writes from multiple processes
	lockFile := c.cacheFile + paths.LockFileSuffix

	// Check for stale locks (locks held by dead/hung processes)
	// If lock file exists and is older than 30 seconds, it's likely stale
	// Note: On Unix, locks are automatically released when a process dies,
	// but if a process hangs, the lock remains. We detect stale locks by checking
	// the lock file's modification time.
	staleLockTimeout := 30 * time.Second
	if stat, err := fileutil.Stat(lockFile); err == nil {
		if time.Since(stat.ModTime()) > staleLockTimeout {
			// Lock file is old - likely from a hung process
			// Try to remove it (safe if process is dead, harmless if process is alive)
			// The actual lock is held by the file descriptor, not the file itself,
			// so removing the file won't break an active lock, but it will allow
			// us to create a new one if the process is truly dead.
			if err := fileutil.Remove(lockFile); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveStaleLock, err).Log()
			}
		}
	}

	file, err := fileutil.OpenFile(lockFile, fileutil.O_CREATE|fileutil.O_RDWR, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf(ErrMsgOpenLockFile).Wrap(err)
	}
	defer file.Close()

	// Bounded retry on LOCK_NB contention. Stale-lock break is attempted mid-loop.
	// TRACK: BLI-REDACTED
	acquireStart := time.Now()
	maxRetries := 50 // Increased maxRetries to 5 seconds to reduce warn-spam
	lockAcquired := false
	for i := 0; i < maxRetries; i++ {
		err = syscallutil.FileFlock(file, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			lockAcquired = true
			break
		}
		if i == 5 {
			if stat, statErr := fileutil.Stat(lockFile); statErr == nil && time.Since(stat.ModTime()) > staleLockTimeout {
				_ = file.Close()
				if rmErr := fileutil.Remove(lockFile); rmErr != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveStaleLock, rmErr).Log()
				}
				file2, err2 := fileutil.OpenFile(lockFile, fileutil.O_CREATE|fileutil.O_RDWR, paths.FilePerm644)
				if err2 == nil {
					file = file2
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	waitMs := time.Since(acquireStart).Milliseconds()
	if waitMs > 10 {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Debug("Validation cache file lock wait").
			Int("wait_ms", int(waitMs)).
			Bool("acquired", lockAcquired).
			Log()
	}
	if !lockAcquired {
		return errfmt.Errorf(ErrMsgCacheLocked)
	}
	now := time.Now()
	if err := fileutil.Chtimes(lockFile, now, now); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgUpdateLockTimestamp, err).Log()
	}
	defer func() {
		if err := syscallutil.FileFlock(file, syscall.LOCK_UN); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Do not overwrite a populated cache with an empty one (e.g. process with no validation
				// state wins the lock and truncates the file that another process had filled).
				Error(ErrMsgReleaseLock, err).Log()
		}
	}()

	if len(byKind) == 0 {
		if stat, statErr := fileutil.Stat(c.cacheFile); statErr == nil && stat.Size() > 0 {
			existingData, readErr := fileutil.ReadFile(c.cacheFile)
			if readErr == nil {
				var existing struct {
					ByKind map[string]json.RawMessage `json:"by_kind"`
				}
				if json.Unmarshal(existingData, &existing) == nil && len(existing.ByKind) > 0 {
					return nil
				}
			}
		}
	} else if shrinkGuardSkipSave(c.cacheFile, byKind) {
		// Refuse to replace a large on-disk warm cache with a tiny in-memory
		// snapshot (classic dual-cache race before AsyncValidator shared the
		// singleton). Empty writes are already guarded above.
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(
			"Skipping validation cache save: in-memory snapshot would shrink on-disk warm cache").
			ProjectRoot(c.projectRoot).
			Log()
		return nil
	} else if staleGenerationSkipSave(c) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(
			"Skipping validation cache save: on-disk generation is newer than this instance's Load").
			ProjectRoot(c.projectRoot).
			Log()
		return nil
	}

	// Rename temp file (atomic write) inside the lock
	if err := fileutil.Rename(tmpFileName, c.cacheFile); err != nil {
		return errfmt.Newf(ErrMsgRenameCacheFile).Wrap(err)
	}

	_ = concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheSaveCopy,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.loadedUpdated = cacheData.Updated
			c.replaceDiskOnNextSave = false
			c.suppressLoadUntilSave = false
			return nil
		},
	)

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

func (c *ValidationStateCache) Invalidate(objectID string) bool {
	var removed bool
	if err := concurrency.RunInLockWithLogger(
		&c.mu,
		LockOpValidationCacheInvalidate,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if _, ok := c.cache[objectID]; ok {
				delete(c.cache, objectID)
				removed = true
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Error(ErrMsgLockFailedInvalidate, err).Log()
	}
	return removed
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
			c.replaceDiskOnNextSave = true
			c.suppressLoadUntilSave = true
			c.loadedUpdated = time.Time{}
			cacheFile = c.cacheFile
			lockFile = c.cacheFile + paths.LockFileSuffix
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Remove cache file if it exists (NO LOCK HELD during I/O)
			Error(ErrMsgLockFailedClear, err).Log()
	}

	if _, err := fileutil.Stat(cacheFile); err == nil {
		if err := fileutil.Remove(cacheFile); err != nil {
			return errfmt.Newf(ErrMsgRemoveCacheFile).Wrap(err)
		}
	}

	// Also remove lock file if it exists
	if _, err := fileutil.Stat(lockFile); err == nil {
		if err := fileutil.Remove(lockFile); err != nil {
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

// shrinkGuardMinDiskEntries is the floor above which we protect a warm on-disk
// validation cache from being replaced by a drastically smaller in-memory snapshot.
const shrinkGuardMinDiskEntries = 200

// shrinkGuardSkipSave returns true when writing byKind would replace a large
// on-disk cache with a tiny snapshot (≤10% of existing entry count).
func shrinkGuardSkipSave(cacheFile string, byKind map[string]kindBucket) bool {
	newCount := 0
	for _, bucket := range byKind {
		newCount += len(bucket.Entries)
	}
	if newCount == 0 {
		return false // empty path uses the separate empty-overwrite guard
	}
	// Substantial snapshots (full/partial checks) may legitimately shrink vs disk;
	// only block tiny CRUD-flusher-sized writes over a warm cache.
	if newCount >= shrinkGuardMinDiskEntries {
		return false
	}
	existingData, err := fileutil.ReadFile(cacheFile)
	if err != nil {
		return false
	}
	var existing struct {
		ByKind map[string]kindBucket `json:"by_kind"`
	}
	if json.Unmarshal(existingData, &existing) != nil || len(existing.ByKind) == 0 {
		return false
	}
	existingCount := 0
	for _, bucket := range existing.ByKind {
		existingCount += len(bucket.Entries)
	}
	if existingCount < shrinkGuardMinDiskEntries {
		return false
	}
	// Skip tiny snapshots that are less than half the warm on-disk size
	// (e.g. ~81 CRUD flusher entries vs thousands from system check).
	return newCount < shrinkGuardMinDiskEntries && newCount*2 < existingCount
}

// staleGenerationSkipSave returns true when another process already wrote a
// newer validation_cache.json than this instance loaded. Without this, a
// long-lived daemon Save of a GhostRef-era snapshot clobbers a CLI
// --clear-cache persist. TRACK: BLI-REDACTED
func staleGenerationSkipSave(c *ValidationStateCache) bool {
	if c == nil {
		return false
	}
	var forceReplace bool
	var loadedUpdated time.Time
	var cacheFile string
	_ = concurrency.RunInRLockWithLogger(
		&c.mu,
		LockOpValidationCacheSaveCopy,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			forceReplace = c.replaceDiskOnNextSave
			loadedUpdated = c.loadedUpdated
			cacheFile = c.cacheFile
			return nil
		},
	)
	if forceReplace {
		return false
	}
	existingData, err := fileutil.ReadFile(cacheFile)
	if err != nil {
		return false
	}
	var existing struct {
		Updated time.Time `json:"updated"`
	}
	if json.Unmarshal(existingData, &existing) != nil || existing.Updated.IsZero() {
		return false
	}
	return existing.Updated.After(loadedUpdated)
}
