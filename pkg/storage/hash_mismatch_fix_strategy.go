package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	"github.com/lanceman/zqk/pkg/when"

	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// HashMismatchFixStrategy defines how hash mismatches should be resolved
// Different strategies can be used for different scenarios (stale index entries, corrupted files, etc.)
type HashMismatchFixStrategy interface {
	// FixHashMismatch attempts to fix a hash mismatch for a given object
	// Returns true if the mismatch was fixed, false if it couldn't be fixed
	FixHashMismatch(ctx context.Context, objectID, kind string, projectRoot string) (bool, error)

	// Name returns a human-readable name for this strategy
	Name() string

	// Description returns a description of what this strategy does
	Description() string
}

// HashMismatchFixResult represents the result of fixing hash mismatches
type HashMismatchFixResult struct {
	ObjectID string
	Kind     string
	Strategy string
	Fixed    bool
	Error    error
	Message  string
	Duration time.Duration
}

// RemoveStaleIndexStrategy removes stale index entries to allow re-indexing
// This is the safest strategy - it removes the stale entry and lets auto-fix re-index
type RemoveStaleIndexStrategy struct {
	Logger logging.Logger
}

// NewRemoveStaleIndexStrategy creates a new remove stale index strategy
func NewRemoveStaleIndexStrategy(logger logging.Logger) *RemoveStaleIndexStrategy {
	return &RemoveStaleIndexStrategy{
		Logger: logger,
	}
}

func (s *RemoveStaleIndexStrategy) Name() string {
	return ConstMiscRemoveStaleIndex
}

func (s *RemoveStaleIndexStrategy) Description() string {
	return ConstMiscRemovesStaleCasIndexEntriesToAllowAutoFi
}

func (s *RemoveStaleIndexStrategy) FixHashMismatch(ctx context.Context, objectID, kind string, projectRoot string) (bool, error) {
	// Get kind directory
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return false, errfmt.Errorf(ConstMiscCannotDetermineDirectoryForKindS, kind)
	}

	// Get CAS instance to access the index
	kindDir := datacell.CellCASPrimaryDir(projectRoot, dirName)
	cas := filecas.NewContentAddressableStorage(kindDir, kind)

	// Get the index
	index := cas.GetIndex()
	if index == nil {
		return false, errfmt.Errorf(ConstMiscCasIndexNotAvailableForKindS, kind)
	}

	// Check if object exists in index
	_, err := index.GetHash(objectID)
	if err != nil {
		// Object not in index - nothing to remove
		StorageLog(s.Logger).Debug(LogEventStorageHashMismatchFixNotInIndexDebug).
			ObjectID(objectID).
			Kind(kind).
			Log()
		return false, nil
	}

	// Remove from index
	if err := index.RemoveMapping(objectID); err != nil {
		return false, errfmt.Newf(ConstMiscFailedToRemoveMapping).Wrap(err)
	}

	StorageLog(s.Logger).Info(LogEventStorageHashMismatchFixRemovedStaleInfo).
		ObjectID(objectID).
		Kind(kind).
		String("strategy", s.Name()).
		Log()

	return true, nil
}

// RecomputeHashStrategy recomputes the hash and updates the index
// This is more aggressive - it recalculates the hash from file content
type RecomputeHashStrategy struct {
	Logger logging.Logger
}

// NewRecomputeHashStrategy creates a new recompute hash strategy
func NewRecomputeHashStrategy(logger logging.Logger) *RecomputeHashStrategy {
	return &RecomputeHashStrategy{
		Logger: logger,
	}
}

func (s *RecomputeHashStrategy) Name() string {
	return ConstMiscRecomputeHash
}

func (s *RecomputeHashStrategy) Description() string {
	return ConstMiscRecomputesHashFromFileContentAndUpdatesT
}

func (s *RecomputeHashStrategy) FixHashMismatch(ctx context.Context, objectID, kind string, projectRoot string) (bool, error) {
	// This strategy would:
	// 1. Find the file (by ID or hash)
	// 2. Read file content
	// 3. Compute hash
	// 4. Update index with new hash
	// 5. Rename file if needed

	// For now, delegate to RemoveStaleIndexStrategy + auto-fix
	// This is safer and leverages existing auto-fix logic
	removeStrategy := NewRemoveStaleIndexStrategy(s.Logger)
	fixed, err := removeStrategy.FixHashMismatch(ctx, objectID, kind, projectRoot)
	if err != nil {
		return false, err
	}

	if fixed {
		StorageLog(s.Logger).Info(LogEventStorageHashMismatchFixRemovedStaleRecomputeInfo).
			ObjectID(objectID).
			Kind(kind).
			Log()
		// Note: Auto-fix will handle the re-indexing
	}

	return fixed, nil
}

// ForceReindexStrategy forces re-indexing by removing entry and immediately re-indexing
// This is the most aggressive strategy
type ForceReindexStrategy struct {
	Logger logging.Logger
}

// NewForceReindexStrategy creates a new force reindex strategy
func NewForceReindexStrategy(logger logging.Logger) *ForceReindexStrategy {
	return &ForceReindexStrategy{
		Logger: logger,
	}
}

func (s *ForceReindexStrategy) Name() string {
	return "force-reindex"
}

func (s *ForceReindexStrategy) Description() string {
	return ConstMiscForcesImmediateReIndexingByRemovingStale
}

func (s *ForceReindexStrategy) FixHashMismatch(ctx context.Context, objectID, kind string, projectRoot string) (bool, error) {
	// Similar to RecomputeHashStrategy, but would also trigger immediate re-index
	// For now, delegate to RemoveStaleIndexStrategy
	// The caller can then run auto-fix to re-index

	removeStrategy := NewRemoveStaleIndexStrategy(s.Logger)
	return removeStrategy.FixHashMismatch(ctx, objectID, kind, projectRoot)
}

// HashMismatchFixer coordinates fixing hash mismatches using strategies
type HashMismatchFixer struct {
	Strategy HashMismatchFixStrategy
	Logger   logging.Logger
}

// NewHashMismatchFixer creates a new hash mismatch fixer with a strategy
func NewHashMismatchFixer(strategy HashMismatchFixStrategy, logger logging.Logger) *HashMismatchFixer {
	return &HashMismatchFixer{
		Strategy: strategy,
		Logger:   logger,
	}
}

// FixHashMismatches fixes hash mismatches for a list of objects
func (f *HashMismatchFixer) FixHashMismatches(ctx context.Context, objectIDs []string, kind string, projectRoot string) ([]HashMismatchFixResult, error) {
	results := make([]HashMismatchFixResult, 0, len(objectIDs))

	for _, objectID := range objectIDs {
		start := time.Now()
		fixed, err := f.Strategy.FixHashMismatch(ctx, objectID, kind, projectRoot)
		duration := time.Since(start)

		result := HashMismatchFixResult{
			ObjectID: objectID,
			Kind:     kind,
			Strategy: f.Strategy.Name(),
			Fixed:    fixed,
			Error:    err,
			Duration: duration,
		}

		when.When(func() bool { return err != nil }).Then(func() {
			result.Message = fmt.Sprintf("Error: %v", err)
		}).OrElseWhen(func() bool { return fixed }).Then(func() {
			result.Message = fmt.Sprintf(ConstMiscFixedUsingSStrategy, f.Strategy.Name())
		}).OrElse(func() {
			result.Message = ConstMiscNoActionNeededObjectNotInIndex
		}).Run()

		results = append(results, result)
	}

	return results, nil
}

// FixHashMismatchesByKind fixes hash mismatches for objects, automatically determining kind from object ID
func (f *HashMismatchFixer) FixHashMismatchesByKind(ctx context.Context, objectIDs []string, projectRoot string) ([]HashMismatchFixResult, error) {
	// Group objects by kind
	objectsByKind := make(map[string][]string)
	for _, objectID := range objectIDs {
		// Infer kind from object ID prefix
		kind := inferKindFromObjectID(objectID)
		if kind == emptyValue {
			StorageLog(f.Logger).Warn(LogEventStorageHashMismatchFixCannotInferKindWarn).
				ObjectID(objectID).
				Log()
			continue
		}
		objectsByKind[kind] = append(objectsByKind[kind], objectID)
	}

	// Fix each kind
	allResults := make([]HashMismatchFixResult, 0)
	for kind, objIDs := range objectsByKind {
		results, err := f.FixHashMismatches(ctx, objIDs, kind, projectRoot)
		if err != nil {
			return nil, errfmt.Errorf(ConstMiscFailedToFixHashMismatchesForKindSW, kind, err)
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

// inferKindFromObjectID attempts to infer the kind from an object ID prefix
func inferKindFromObjectID(objectID string) string {
	// Extract prefix (e.g., "BLI-123" -> "BLI")
	prefix := ""
	for i, r := range objectID {
		if r == '-' {
			prefix = objectID[:i]
			break
		}
	}

	if prefix == emptyValue {
		return ""
	}

	// Map common prefixes to kinds
	prefixToKind := map[string]string{
		"AAM":  objects.KindAuditAggregationMetric,
		"AUD":  objects.KindAuditEvent,
		"SHM":  objects.KindSchedulerHealthMetric,
		"FLM":  objects.KindFileLockMetric,
		"TEST": objects.KindTestCase,
		"POL":  objects.KindPolicy,
		"CRIT": objects.KindCriteria,
		"BLI":  objects.KindBacklogItem,
		"REQ":  objects.KindRequirement,
		"MIL":  objects.KindMilestone,
		"GOAL": objects.KindGoal,
		"WS":   objects.KindWorkstream,
		"PRIO": objects.KindPriorityPlan,
		"ADR":  objects.KindDecision,
		"AUTH": objects.KindAuthStrategy,
		"KEY":  objects.KindKeystoreEntry,
		"SCH":  objects.KindSchedulerJob,
		"DOC":  objects.KindDocEntry,
	}

	if kind, ok := prefixToKind[prefix]; ok {
		return kind
	}

	// Fallback: try to get kind from directory mapping
	// This is more robust but requires scanning
	return ""
}
