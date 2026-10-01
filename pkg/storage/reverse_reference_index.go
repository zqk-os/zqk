package storage

import (
	stdcontext "context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/scanner"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	reverseReferenceIndexFile    = "reverse-reference-index.json"
	reverseReferenceIndexVersion = "1.0"
)

// ReverseReferenceIndexMetadata holds metadata about the reverse reference index cache
type ReverseReferenceIndexMetadata struct {
	Version     string    `json:"version"`
	BuildTime   time.Time `json:"build_time"`
	ProjectRoot string    `json:"project_root"`
	EntryCount  int       `json:"entry_count"`
}

// ReverseReferenceIndex is a thread-safe cache for reverse references (referencedID -> []dependentIDs)
type ReverseReferenceIndex struct {
	mu           sync.RWMutex
	index        map[string][]string // referencedID -> []dependentIDs
	forwardIndex map[string][]string // objectID -> []referencedIDs
	metadata     *ReverseReferenceIndexMetadata
	cacheDir     string // Directory where cache file is stored
	projectRoot  string // Associated project root directory
	isReady      atomic.Bool
}

// Global cache instance and project-scoped registry
var (
	globalReverseReferenceIndex *ReverseReferenceIndex
	reverseReferenceIndexOnce   sync.Once

	projectScopedIndices   = make(map[string]*ReverseReferenceIndex)
	projectScopedIndicesMu sync.RWMutex
)

// GetReverseReferenceIndexForProject returns the isolated reverse reference index for the specified project root.
func GetReverseReferenceIndexForProject(projectRoot string) *ReverseReferenceIndex {
	normalized := filepath.Clean(projectRoot)
	if projectRoot == "" {
		normalized = ""
	}

	projectScopedIndicesMu.RLock()
	idx, exists := projectScopedIndices[normalized]
	projectScopedIndicesMu.RUnlock()
	if exists && idx != nil {
		return idx
	}

	projectScopedIndicesMu.Lock()
	defer projectScopedIndicesMu.Unlock()
	if idx, exists := projectScopedIndices[normalized]; exists && idx != nil {
		return idx
	}

	idx = NewReverseReferenceIndexForProject(normalized)
	projectScopedIndices[normalized] = idx
	return idx
}

// GetGlobalReverseReferenceIndex returns the global reverse reference index instance.
// Deprecated: Prefer GetReverseReferenceIndexForProject(projectRoot) or explicit dependency injection (TDE-CEF-F-ARCH-003).
func GetGlobalReverseReferenceIndex() *ReverseReferenceIndex {
	reverseReferenceIndexOnce.Do(func() {
		globalReverseReferenceIndex = NewReverseReferenceIndex()
	})
	return globalReverseReferenceIndex
}

// NewReverseReferenceIndex creates a new reverse reference index without a project root.
// Deprecated: Prefer NewReverseReferenceIndexForProject(projectRoot) for project isolation (TDE-CEF-F-ARCH-003).
func NewReverseReferenceIndex() *ReverseReferenceIndex {
	return NewReverseReferenceIndexForProject("")
}

// NewReverseReferenceIndexForProject creates a new reverse reference index bound to an explicit project root.
func NewReverseReferenceIndexForProject(projectRoot string) *ReverseReferenceIndex {
	normalized := filepath.Clean(projectRoot)
	if projectRoot == "" {
		normalized = ""
	}
	return &ReverseReferenceIndex{
		index:        make(map[string][]string),
		forwardIndex: make(map[string][]string),
		metadata:     nil,
		cacheDir:     "",
		projectRoot:  normalized,
	}
}

// ProjectRoot returns the project root directory associated with this index instance.
func (r *ReverseReferenceIndex) ProjectRoot() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.projectRoot
}

// IsReady returns true if the index has been successfully loaded from cache or built from scan.
func (r *ReverseReferenceIndex) IsReady() bool {
	return r.isReady.Load()
}

// GetCacheFilePath returns the path to the cache file for a project root (for reporting).
func (r *ReverseReferenceIndex) GetCacheFilePath(projectRoot string) string {
	return r.getCacheFilePath(projectRoot)
}

// getCacheFilePath returns the path to the cache file.
// When projectRoot is non-empty, always use it so SaveCache(projectRoot) writes to the correct project.
func (r *ReverseReferenceIndex) getCacheFilePath(projectRoot string) string {
	targetRoot := projectRoot
	if targetRoot == emptyValue {
		r.mu.RLock()
		targetRoot = r.projectRoot
		r.mu.RUnlock()
	}
	if targetRoot != emptyValue {
		cacheDir := filepath.Join(targetRoot, paths.ProjectDataDir, paths.CacheDir)
		return filepath.Join(cacheDir, reverseReferenceIndexFile)
	}
	r.mu.RLock()
	dir := r.cacheDir
	r.mu.RUnlock()
	if dir != emptyValue {
		return filepath.Join(dir, reverseReferenceIndexFile)
	}
	return ""
}

// LoadCache loads the cache from disk if it exists and is still valid
// Returns true if cache was successfully loaded, false if cache needs to be rebuilt
func newReverseRefIndexLockContext() (stdcontext.Context, stdcontext.CancelFunc) {
	return stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
}

func newReverseRefIndexLockLogger() logging.Logger {
	return logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
}

func (r *ReverseReferenceIndex) initMapsLocked() {
	if r.index == nil {
		r.index = make(map[string][]string)
	}
	if r.forwardIndex == nil {
		r.forwardIndex = make(map[string][]string)
	}
}

func (r *ReverseReferenceIndex) withReadLock(lockName string, op func() error) error {
	ctx, cancel := newReverseRefIndexLockContext()
	defer cancel()
	return concurrency.WithRLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(newReverseRefIndexLockLogger()),
		lockName,
		op,
	)
}

func (r *ReverseReferenceIndex) withWriteLock(lockName string, op func() error) error {
	ctx, cancel := newReverseRefIndexLockContext()
	defer cancel()
	return concurrency.WithLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(newReverseRefIndexLockLogger()),
		lockName,
		op,
	)
}

// ReferencedIDCount returns the total count of referenced IDs tracked in the index.
func (r *ReverseReferenceIndex) ReferencedIDCount() int {
	var count int
	if err := r.withReadLock(locknames.LockNameReverseReferenceIndexReferencedIDCount, func() error {
		if r.index != nil {
			count = len(r.index)
		}
		return nil
	}); err != nil {
		logging.LogSwallowedError(err)
	}
	return count
}

// GetDependentsWithContext retrieves all dependent object IDs with fail-closed error propagation
// using the provided context for lock acquisition timeout.
func (r *ReverseReferenceIndex) GetDependentsWithContext(ctx stdcontext.Context, referencedID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var dependents []string
	err := concurrency.WithRLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexGetDependents,
		func() error {
			if r.index == nil {
				return nil
			}
			deps, exists := r.index[referencedID]
			if exists {
				dependents = make([]string, len(deps))
				copy(dependents, deps)
			}
			return nil
		},
	)
	if err != nil {
		return nil, errfmt.Errorf("reverse reference index lock timeout getting dependents for %s: %w", referencedID, err)
	}
	return dependents, nil
}

// GetDependentsWithError retrieves all dependent object IDs with fail-closed error propagation
// if the read lock times out or cannot be acquired.
func (r *ReverseReferenceIndex) GetDependentsWithError(referencedID string) ([]string, error) {
	ctx, cancel := newReverseRefIndexLockContext()
	defer cancel()
	return r.GetDependentsWithContext(ctx, referencedID)
}

func (r *ReverseReferenceIndex) GetDependents(referencedID string) []string {
	deps, err := r.GetDependentsWithError(referencedID)
	if err != nil {
		logging.LogSwallowedError(err)
		return nil
	}
	return deps
}

func (r *ReverseReferenceIndex) AddReference(objectID, referencedID string) {
	if referencedID == emptyValue || objectID == emptyValue {
		return
	}
	err := r.withWriteLock(locknames.LockNameReverseReferenceIndexAddReference, func() error {
		r.initMapsLocked()
		r.addReferenceLocked(objectID, referencedID)
		return nil
	})
	if err != nil {
		logging.LogSwallowedError(err)
	}
}

// RemoveReference removes a reference relationship (objectID no longer references referencedID)
func (r *ReverseReferenceIndex) RemoveReference(objectID, referencedID string) {
	if referencedID == emptyValue || objectID == emptyValue {
		return
	}
	err := r.withWriteLock(locknames.LockNameReverseReferenceIndexRemoveReference, func() error {
		if r.index == nil {
			return nil
		}
		r.removeReferenceLocked(objectID, referencedID)
		return nil
	})
	if err != nil {
		logging.LogSwallowedError(err)
	}
}

// RemoveObject removes all references for an object (when object is deleted)
func (r *ReverseReferenceIndex) RemoveObject(objectID string) {
	if objectID == emptyValue {
		return
	}
	err := r.withWriteLock(locknames.LockNameReverseReferenceIndexRemoveObject, func() error {
		if r.index == nil {
			return nil
		}

		if r.forwardIndex != nil {
			refs := r.forwardIndex[objectID]
			for _, referencedID := range refs {
				deps := r.index[referencedID]
				newDeps := make([]string, 0, len(deps))
				for _, dep := range deps {
					if dep != objectID {
						newDeps = append(newDeps, dep)
					}
				}
				if len(newDeps) == 0 {
					delete(r.index, referencedID)
				} else {
					r.index[referencedID] = newDeps
				}
			}
			delete(r.forwardIndex, objectID)
			return nil
		}

		// Fallback: full scan only if forward index structure was not initialized
		for referencedID, deps := range r.index {
			found := false
			for _, dep := range deps {
				if dep == objectID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
			newDeps := make([]string, 0, len(deps)-1)
			for _, dep := range deps {
				if dep != objectID {
					newDeps = append(newDeps, dep)
				}
			}
			if len(newDeps) == 0 {
				delete(r.index, referencedID)
			} else {
				r.index[referencedID] = newDeps
			}
		}
		return nil
	})
	if err != nil {
		logging.LogSwallowedError(err)
	}
}

// UpdateReferences updates references for an object (removes old refs, adds new refs)
func (r *ReverseReferenceIndex) UpdateReferences(objectID string, oldRefs, newRefs []string) {
	if objectID == emptyValue {
		return
	}
	err := r.withWriteLock(locknames.LockNameReverseReferenceIndexUpdateReferences, func() error {
		r.initMapsLocked()

		for _, oldRef := range oldRefs {
			if oldRef != emptyValue {
				r.removeReferenceLocked(objectID, oldRef)
			}
		}

		for _, newRef := range newRefs {
			if newRef != emptyValue {
				r.addReferenceLocked(objectID, newRef)
			}
		}
		return nil
	})
	if err != nil {
		logging.LogSwallowedError(err)
	}
}

// removeReferenceLocked removes a reference (must be called with lock held)
func (r *ReverseReferenceIndex) removeReferenceLocked(objectID, referencedID string) {
	deps := r.index[referencedID]
	newDeps := make([]string, 0, len(deps))
	for _, dep := range deps {
		if dep != objectID {
			newDeps = append(newDeps, dep)
		}
	}
	if len(newDeps) == 0 {
		delete(r.index, referencedID)
	} else {
		r.index[referencedID] = newDeps
	}

	if r.forwardIndex != nil {
		refs := r.forwardIndex[objectID]
		newRefs := make([]string, 0, len(refs))
		for _, ref := range refs {
			if ref != referencedID {
				newRefs = append(newRefs, ref)
			}
		}
		if len(newRefs) == 0 {
			delete(r.forwardIndex, objectID)
		} else {
			r.forwardIndex[objectID] = newRefs
		}
	}
}

// addReferenceLocked adds a reference (must be called with lock held)
func (r *ReverseReferenceIndex) addReferenceLocked(objectID, referencedID string) {
	deps := r.index[referencedID]
	// Check if already exists
	alreadyDep := false
	for _, dep := range deps {
		if dep == objectID {
			alreadyDep = true
			break
		}
	}
	if !alreadyDep {
		r.index[referencedID] = append(deps, objectID)
	}

	if r.forwardIndex == nil {
		r.forwardIndex = make(map[string][]string)
	}
	refs := r.forwardIndex[objectID]
	alreadyRef := false
	for _, ref := range refs {
		if ref == referencedID {
			alreadyRef = true
			break
		}
	}
	if !alreadyRef {
		r.forwardIndex[objectID] = append(refs, referencedID)
	}
}

// Clear clears the entire index (for rebuild)
func (r *ReverseReferenceIndex) Clear() {
	if err := r.withWriteLock(locknames.LockNameReverseReferenceIndexClear, func() error {
		r.index = make(map[string][]string)
		r.forwardIndex = make(map[string][]string)
		r.metadata = nil
		r.isReady.Store(false)
		return nil
	}); err != nil {
		logging.LogSwallowedError(err)
	}
}

// BuildFromScan populates the index by scanning all object YAML files under processDir for the given kinds.
// Clear is implied at the start so the index is fully replaced. Used when LoadCache returns false
// (e.g. cold start or cache invalid). projectRoot is used for cache path; processDir should be
// datacell.ProcessPrimaryDir(projectRoot).
func (r *ReverseReferenceIndex) BuildFromScan(projectRoot, processDir string, kinds []string) error {
	r.Clear()
	if projectRoot != emptyValue {
		r.mu.Lock()
		if r.projectRoot == emptyValue {
			r.projectRoot = filepath.Clean(projectRoot)
		}
		r.mu.Unlock()
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	for _, kind := range kinds {
		dirName := objects.GetDirectoryFromKind(kind)
		if dirName == emptyValue {
			continue
		}
		kindDir := filepath.Join(processDir, dirName)
		scnr := scanner.NewYAMLScanner(kindDir)
		files, err := scnr.Scan()
		if err != nil {
			if fileutil.IsNotExist(err) {
				continue
			}
			// Log and continue so one missing or unreadable kind dir does not abort the whole build
			StorageLog(logger).Debug(LogEventStorageReverseRefIndexSkipKindDirDebug).
				String("kind_dir", kindDir).
				Kind(kind).
				WithError(err).
				Log()
			continue
		}
		for _, file := range files {
			data, err := fileutil.ReadFile(file.Path)
			if err != nil {
				continue
			}
			var obj map[string]any
			if err := yaml.Unmarshal(data, &obj); err != nil {
				continue
			}
			id, _ := obj[objects.FieldKeyID].(string)
			if id == emptyValue {
				id = file.ObjectID
			}
			if id == emptyValue {
				continue
			}
			updateReverseReferenceIndexOnCreate(id, obj)
		}
	}
	r.isReady.Store(true)

	StorageLog(logger).Debug(LogEventStorageReverseRefIndexBuiltFromScanDebug).
		ProjectRoot(projectRoot).
		Int("kinds", len(kinds)).
		Log()
	return nil
}

// GetReferencedObjectIDs returns all object IDs referenced by the given object (one level).
// Used by lifecycle dependency propagation to walk one level up (refs) or down (dependents).
func (r *ReverseReferenceIndex) GetReferencedObjectIDs(objectID string) []string {
	if objectID == emptyValue {
		return nil
	}
	var refs []string
	if err := r.withReadLock(locknames.LockNameReverseReferenceIndexGetReferencedIDs, func() error {
		if r.forwardIndex == nil {
			return nil
		}
		if list, exists := r.forwardIndex[objectID]; exists {
			refs = make([]string, len(list))
			copy(refs, list)
		}
		return nil
	}); err != nil {
		logging.LogSwallowedError(err)
		return nil
	}
	return refs
}
