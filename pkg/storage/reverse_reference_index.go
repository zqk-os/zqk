package storage

import (
	stdcontext "context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/migration/scanner"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
	"gopkg.in/yaml.v3"
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
	mu       sync.RWMutex
	index    map[string][]string // referencedID -> []dependentIDs
	metadata *ReverseReferenceIndexMetadata
	cacheDir string // Directory where cache file is stored
	isReady  atomic.Bool
}

// Global cache instance (similar to ObjectIDCache pattern)
var (
	globalReverseReferenceIndex *ReverseReferenceIndex
	reverseReferenceIndexOnce   sync.Once
)

// GetGlobalReverseReferenceIndex returns the global reverse reference index instance
func GetGlobalReverseReferenceIndex() *ReverseReferenceIndex {
	reverseReferenceIndexOnce.Do(func() {
		globalReverseReferenceIndex = NewReverseReferenceIndex()
	})
	return globalReverseReferenceIndex
}

// NewReverseReferenceIndex creates a new reverse reference index
func NewReverseReferenceIndex() *ReverseReferenceIndex {
	return &ReverseReferenceIndex{
		index:    make(map[string][]string),
		metadata: nil,
		cacheDir: "",
	}
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
	if projectRoot != emptyValue {
		cacheDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir)
		return filepath.Join(cacheDir, reverseReferenceIndexFile)
	}
	if r.cacheDir != emptyValue {
		return filepath.Join(r.cacheDir, reverseReferenceIndexFile)
	}
	return ""
}

// LoadCache loads the cache from disk if it exists and is still valid
// Returns true if cache was successfully loaded, false if cache needs to be rebuilt
func (r *ReverseReferenceIndex) LoadCache(projectRoot string) (bool, error) {
	cachePath := r.getCacheFilePath(projectRoot)
	r.cacheDir = filepath.Dir(cachePath)

	// Check if cache file exists
	data, err := os.ReadFile(cachePath)
	if os.IsNotExist(err) {
		// Cache doesn't exist - will be built fresh
		return false, nil
	}
	if err != nil {
		return false, errfmt.Newf(ConstMiscFailedToReadCacheFile).Wrap(err)
	}

	// Parse cache file
	var cacheData struct {
		Metadata *ReverseReferenceIndexMetadata `json:"metadata"`
		Index    map[string][]string            `json:"index"`
	}
	if err := json.Unmarshal(data, &cacheData); err != nil {
		// Cache file is corrupted - will be rebuilt
		return false, nil
	}

	// Validate cache metadata
	if cacheData.Metadata == nil {
		// Invalid cache - will be rebuilt
		return false, nil
	}

	// Check if project root matches
	if cacheData.Metadata.ProjectRoot != projectRoot {
		// Different project - cache is invalid
		return false, nil
	}

	// Check version compatibility
	if cacheData.Metadata.Version != reverseReferenceIndexVersion {
		// Version mismatch - cache is invalid
		return false, nil
	}

	// Cache is valid - load it (after I/O operations)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var entryCount int
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_116 = concurrency.WithLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexLoad,
		func() error {
			if r.index == nil {
				r.index = make(map[string][]string)
			}
			r.index = cacheData.Index
			r.metadata = cacheData.Metadata
			r.isReady.Store(true)
			entryCount = len(r.index)
			return nil
		},
	)
	if err_swallow_116 != nil {
		logging.LogSwallowedError(err_swallow_116)
	}

	StorageLog(logger).Debug(LogEventStorageReverseRefIndexLoadedDebug).
		EntryCount(entryCount).
		BuildTime(zqktime.FormatRFC3339UTC(cacheData.Metadata.BuildTime)).
		Log()

	return true, nil
}

// SaveCache saves the cache to disk
func (r *ReverseReferenceIndex) SaveCache(projectRoot string) error {
	saveStart := time.Now()
	var entryCount int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_117 = concurrency.WithRLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexSaveGetCount,
		func() error {
			entryCount = len(r.index)
			return nil
		},
	)
	if err_swallow_117 != nil {
		logging.LogSwallowedError(err_swallow_117)
	}

	cachePath := r.getCacheFilePath(projectRoot)
	cacheDir := filepath.Dir(cachePath)

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ConstMiscFailedToCreateCacheDirectory).Wrap(err)
	}

	// Update metadata and prepare cache data
	var cacheData struct {
		Metadata *ReverseReferenceIndexMetadata `json:"metadata"`
		Index    map[string][]string            `json:"index"`
	}
	ctx2, cancel2 := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel2()
	var err_swallow_118 = concurrency.WithLockTimeout(
		&r.mu,
		ctx2,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexSavePrepare,
		func() error {
			r.metadata = &ReverseReferenceIndexMetadata{
				Version:     reverseReferenceIndexVersion,
				BuildTime:   time.Now(),
				ProjectRoot: projectRoot,
				EntryCount:  entryCount,
			}

			cacheData.Index = make(map[string][]string, len(r.index))
			for k, v := range r.index {

				dependents := make([]string, len(v))
				copy(dependents, v)
				cacheData.Index[k] = dependents
			}
			cacheData.Metadata = r.metadata
			return nil
		},
	)
	if err_swallow_118 !=

		// Marshal to JSON
		nil {
		logging.LogSwallowedError(err_swallow_118)
	}

	data, err := json.MarshalIndent(cacheData, "", "  ")
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToMarshalCache).Wrap(err)
	}

	// Add trailing newline
	data = append(data, '\n')

	// Write to file
	if err := os.WriteFile(cachePath, data, paths.FilePerm644); err != nil { //nolint:gosec // Cache files - 0600 is acceptable
		return errfmt.Newf(ConstMiscFailedToWriteCacheFile).Wrap(err)
	}

	StorageLog(logger).Debug(LogEventStorageReverseRefIndexSavedDebug).
		EntryCount(entryCount).
		ElapsedString(time.Since(saveStart).Round(time.Millisecond).String()).
		Log()

	return nil
}

// ReferencedIDCount returns the number of referenced IDs in the index (for reporting).
func (r *ReverseReferenceIndex) ReferencedIDCount() int {
	var count int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_119 = concurrency.WithRLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexReferencedIDCount,
		func() error {
			if r.index != nil {
				count = len(r.index)
			}
			return nil
		},
	)
	if err_swallow_119 !=

		// GetDependents returns all objects that reference the given ID
		// Returns empty slice if no dependents found
		nil {
		logging.LogSwallowedError(err_swallow_119)
	}
	return count
}

func (r *ReverseReferenceIndex) GetDependents(referencedID string) []string {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var dependents []string
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_120 = concurrency.WithRLockTimeout(
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
	if err_swallow_120 != nil {
		logging.

			// AddReference adds a reference relationship (objectID references referencedID)
			LogSwallowedError(err_swallow_120)
	}
	return dependents
}

func (r *ReverseReferenceIndex) AddReference(objectID, referencedID string) {
	if referencedID == emptyValue || objectID == emptyValue {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_121 = concurrency.WithLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexAddReference,
		func() error {
			if r.index == nil {
				r.index = make(map[string][]string)
			}

			deps := r.index[referencedID]
			for _, dep := range deps {
				if dep == objectID {

					return nil
				}
			}

			r.index[referencedID] = append(deps, objectID)
			return nil
		},
	)
	if err_swallow_121 !=

		// RemoveReference removes a reference relationship (objectID no longer references referencedID)
		nil {
		logging.LogSwallowedError(err_swallow_121)
	}
}

func (r *ReverseReferenceIndex) RemoveReference(objectID, referencedID string) {
	if referencedID == emptyValue || objectID == emptyValue {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_122 = concurrency.WithLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexRemoveReference,
		func() error {
			if r.index == nil {
				return nil
			}
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
			return nil
		},
	)
	if err_swallow_122 !=

		// RemoveObject removes all references for an object (when object is deleted)
		nil {
		logging.LogSwallowedError(err_swallow_122)
	}
}

func (r *ReverseReferenceIndex) RemoveObject(objectID string) {
	if objectID == emptyValue {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_123 = concurrency.WithLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexRemoveObject,
		func() error {
			if r.index == nil {
				return nil
			}

			for referencedID, deps := range r.index {
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
			return nil
		},
	)
	if err_swallow_123 !=

		// UpdateReferences updates references for an object (removes old refs, adds new refs)
		nil {
		logging.LogSwallowedError(err_swallow_123)
	}
}

func (r *ReverseReferenceIndex) UpdateReferences(objectID string, oldRefs, newRefs []string) {
	if objectID == emptyValue {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_124 = concurrency.WithLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexUpdateReferences,
		func() error {
			if r.index == nil {
				r.index = make(map[string][]string)
			}

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
		},
	)
	if err_swallow_124 !=

		// removeReferenceLocked removes a reference (must be called with lock held)
		nil {
		logging.LogSwallowedError(err_swallow_124)
	}
}

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
}

// addReferenceLocked adds a reference (must be called with lock held)
func (r *ReverseReferenceIndex) addReferenceLocked(objectID, referencedID string) {
	deps := r.index[referencedID]
	// Check if already exists
	for _, dep := range deps {
		if dep == objectID {
			return
		}
	}
	// Add to list
	r.index[referencedID] = append(deps, objectID)
}

// Clear clears the entire index (for rebuild)
func (r *ReverseReferenceIndex) Clear() {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_125 = concurrency.WithLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexClear,
		func() error {
			r.index = make(map[string][]string)
			r.metadata = nil
			r.isReady.Store(false)
			return nil
		},
	)
	if err_swallow_125 !=

		// BuildFromScan populates the index by scanning all object YAML files under processDir for the given kinds.
		// Clear is implied at the start so the index is fully replaced. Used when LoadCache returns false
		// (e.g. cold start or cache invalid). projectRoot is used for cache path; processDir should be
		// datacell.ProcessPrimaryDir(projectRoot).
		nil {
		logging.LogSwallowedError(err_swallow_125)
	}
}

func (r *ReverseReferenceIndex) BuildFromScan(projectRoot, processDir string, kinds []string) error {
	r.Clear()
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
			if os.IsNotExist(err) {
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
			data, err := os.ReadFile(file.Path)
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
func GetReferencedObjectIDs(obj map[string]any) []string {
	return extractReferenceIDsFromObject(obj)
}

// extractReferenceIDsFromObject extracts all referenced object IDs from an object
// Returns a slice of object IDs that this object references
func extractReferenceIDsFromObject(obj map[string]any) []string {
	if obj == nil {
		return nil
	}
	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(obj)
	var refIDs []string
	for fieldName, refValue := range refFields {
		if refValue == nil {
			continue
		}
		// Skip commit_refs (Git commit hashes, not object IDs)
		if fieldName == "commit_refs" {
			continue
		}
		// Extract reference IDs from various formats
		switch v := refValue.(type) {
		case string:
			if v != emptyValue {
				refIDs = append(refIDs, extractObjectIDFromReference(v))
			}
		case []any:
			for _, item := range v {
				if str, ok := item.(string); ok && str != emptyValue {
					refIDs = append(refIDs, extractObjectIDFromReference(str))
				}
			}
		case []string:
			for _, str := range v {
				if str != emptyValue {
					refIDs = append(refIDs, extractObjectIDFromReference(str))
				}
			}
		}
	}
	// Remove duplicates and empty strings
	seen := make(map[string]bool)
	result := make([]string, 0, len(refIDs))
	for _, id := range refIDs {
		if id != emptyValue && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// extractObjectIDFromReference extracts the object ID from a reference string
// Handles various formats: "ITEM-001", "backlog_item:ITEM-001", "domain:process:backlog_item:ITEM-001", "account:username"
func extractObjectIDFromReference(refStr string) string {
	if refStr == emptyValue {
		return ""
	}
	// Parse namespace format (e.g., "domain:process:backlog_item:ITEM-002")
	parsed := validation.ParseNamespace(refStr)
	if parsed != nil && parsed.ObjectID != emptyValue {
		return parsed.ObjectID
	}
	// Handle "kind:id" format (e.g., "backlog_item:ITEM-002")
	if strings.Contains(refStr, ":") {
		parts := strings.SplitN(refStr, ":", 2)
		if len(parts) == 2 {
			// Check if it's an account reference (keep full format)
			if parts[0] == objects.KindAccount {
				return refStr // Keep "account:username" format
			}
			return parts[1] // Return just the ID part
		}
		// Full namespace format (e.g., "domain:process:backlog_item:ITEM-002")
		parts = strings.Split(refStr, ":")
		if len(parts) > 0 {
			return parts[len(parts)-1] // Last part is the ID
		}
	}
	// Simple format (e.g., "ITEM-002")
	return refStr
}

// updateReverseReferenceIndexOnCreate updates the reverse reference index when an object is created
func updateReverseReferenceIndexOnCreate(objectID string, obj map[string]any) {
	if objectID == emptyValue || obj == nil {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	refIDs := extractReferenceIDsFromObject(obj)
	for _, refID := range refIDs {
		if refID != emptyValue {
			index.AddReference(objectID, refID)
		}
	}
}

// updateReverseReferenceIndexOnUpdate updates the reverse reference index when an object is updated
func updateReverseReferenceIndexOnUpdate(objectID string, oldObj, newObj map[string]any) {
	if objectID == emptyValue {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	var oldRefIDs []string
	if oldObj != nil {
		oldRefIDs = extractReferenceIDsFromObject(oldObj)
	}
	var newRefIDs []string
	if newObj != nil {
		newRefIDs = extractReferenceIDsFromObject(newObj)
	}
	index.UpdateReferences(objectID, oldRefIDs, newRefIDs)
}

// updateReverseReferenceIndexOnDelete updates the reverse reference index when an object is deleted
func updateReverseReferenceIndexOnDelete(objectID string) {
	if objectID == emptyValue {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	index.RemoveObject(objectID)
}

// updateReverseReferenceIndexOnIDChange updates the reverse reference index when an object's ID changes
func updateReverseReferenceIndexOnIDChange(oldID, newID string, obj map[string]any) {
	if oldID == emptyValue || newID == emptyValue || oldID == newID {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	// Remove old ID from all dependent lists
	index.RemoveObject(oldID)
	// Add new ID with same references
	refIDs := extractReferenceIDsFromObject(obj)
	for _, refID := range refIDs {
		if refID != emptyValue {
			index.AddReference(newID, refID)
		}
	}
}
