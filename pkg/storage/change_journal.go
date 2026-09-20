package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

type changeJournalTracker struct {
	wg      sync.WaitGroup
	pending atomic.Int64
}

var (
	changeJournalTrackers   = make(map[string]*changeJournalTracker)
	changeJournalTrackersMu sync.RWMutex
)

func getChangeJournalTracker(projectRoot string) *changeJournalTracker {
	if projectRoot == emptyValue {
		projectRoot = "global"
	}
	changeJournalTrackersMu.RLock()
	t, ok := changeJournalTrackers[projectRoot]
	changeJournalTrackersMu.RUnlock()
	if ok {
		return t
	}
	changeJournalTrackersMu.Lock()
	defer changeJournalTrackersMu.Unlock()
	if t, ok := changeJournalTrackers[projectRoot]; ok {
		return t
	}
	t = &changeJournalTracker{}
	changeJournalTrackers[projectRoot] = t
	return t
}

// ChangeJournalShutdownHandler coordinates graceful drain of change journal writes during shutdown.
type ChangeJournalShutdownHandler struct{}

// InitiateShutdown implements QueueShutdownHandler
func (h *ChangeJournalShutdownHandler) InitiateShutdown() error {
	return nil
}

// Drain implements QueueShutdownHandler
func (h *ChangeJournalShutdownHandler) Drain(ctx context.Context) error {
	return DrainChangeJournal(ctx)
}

// IsDrained implements QueueShutdownHandler
func (h *ChangeJournalShutdownHandler) IsDrained() bool {
	return h.GetPendingCount() == 0
}

// GetPendingCount implements QueueShutdownHandler
func (h *ChangeJournalShutdownHandler) GetPendingCount() int64 {
	changeJournalTrackersMu.RLock()
	defer changeJournalTrackersMu.RUnlock()
	var total int64
	for _, t := range changeJournalTrackers {
		total += t.pending.Load()
	}
	return total
}

// GetName implements QueueShutdownHandler
func (h *ChangeJournalShutdownHandler) GetName() string {
	return "change_journal"
}

// IsCritical implements QueueShutdownHandler
func (h *ChangeJournalShutdownHandler) IsCritical() bool {
	return true
}

func init() {
	GetGlobalShutdownCoordinator().RegisterQueue(&ChangeJournalShutdownHandler{})
}

// DrainChangeJournalForRoot blocks until all in-flight asynchronous change journal writes
// for the given projectRoot complete, or until ctx is cancelled.
func DrainChangeJournalForRoot(ctx context.Context, projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	t := getChangeJournalTracker(projectRoot)
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("storage-change-journal-drain", "wait for change journal writes").
		StartSimple(func() {
			t.wg.Wait()
			close(done)
		})
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DrainChangeJournal blocks until all in-flight asynchronous change journal writes
// across all project roots complete, or until ctx is cancelled.
func DrainChangeJournal(ctx context.Context) error {
	changeJournalTrackersMu.RLock()
	roots := make([]string, 0, len(changeJournalTrackers))
	for root := range changeJournalTrackers {
		roots = append(roots, root)
	}
	changeJournalTrackersMu.RUnlock()

	for _, root := range roots {
		if err := DrainChangeJournalForRoot(ctx, root); err != nil {
			return err
		}
	}
	return nil
}

// findNextChangeJournalID finds the next available change journal entry ID.
// When stream storage is enabled for change_journal_entry, uses a per-project batch cache
// that allocates IDs from the sequence file in batches to reduce flock contention (see
// change_journal_id_cache.go and SCHEDULER_SAMPLE_FLOCK_AND_JOB_SETUP_20260313.md).
func findNextChangeJournalID(ctx context.Context, projectRoot, journalDir string) (string, error) {
	if projectRoot != emptyValue && StreamStorageEnabledForKind(objects.KindChangeJournalEntry) {
		cache := getChangeJournalIDCache(ctx, projectRoot)
		return cache.nextID(ctx)
	}
	// Non-stream path: use thread-safe batch generator with cross-process file locking
	generator := getFallbackChangeJournalGenerator(ctx, projectRoot, journalDir)
	return generator.GenerateNextID()
}

// createChangeJournalEntry creates a change journal entry for an object update
// This provides a complete audit trail of object changes for rollback and auditing
//
//nolint:unparam // error always nil - best effort function that never fails
func createChangeJournalEntry(parentCtx context.Context, projectRoot, id, kind, _, changeType string, previousState, updates map[string]any, secCtx *pkgctx.SecurityContext, fileStorage *FileObjectStorage) error {
	if kind == ConstMiscPromptTemplate {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Debug(ConstMiscCreatechangejournalentryCalled).Kind(kind).ObjectID(id).ChangeType(changeType).Log()
	}
	if projectRoot == emptyValue {
		// Can't create change journal entry without project root
		return nil // Best effort - don't fail update
	}
	if StreamStorageEnabledForKind(kind) {
		return nil // HV create/update is itself the audit trail; journaling it doubles stream write-amp.
	}
	if kind == objects.KindChangeJournalEntry || kind == objects.KindAuditEvent {
		return nil // Don't track changes to audit events or the change journal itself
	}

	// Build object reference (format: "kind:id")
	objectRef := fmt.Sprintf("%s:%s", kind, id)

	// Build diff summary
	diffSummary := buildDiffSummary(updates, previousState)

	// Flatten updates to dotted paths for analysis and future compaction (e.g. dictionary compression)
	changedPaths := flattenUpdatePaths(updates)

	// Build title from change type and object reference
	title := fmt.Sprintf("%s: %s", strings.Title(changeType), objectRef)

	// Build options for change journal entry creation
	options := &ChangeJournalEntryOptions{
		ChangeType:    changeType,
		ObjectRef:     objectRef,
		DiffSummary:   diffSummary,
		ChangedPaths:  changedPaths,
		Title:         title,
		PreviousState: nil, // Set below if needed
		CreatedBy:     secCtx.AccountID,
	}

	// Include previous state for rollback capability (only for updates and deletes)
	if changeType == OpUpdate || changeType == OpDelete {
		if previousState != nil {
			// Create a copy to avoid modifying the original
			previousStateCopy := make(map[string]any)
			for k, v := range previousState {
				previousStateCopy[k] = v
			}
			options.PreviousState = previousStateCopy
		}
	}

	tracker := getChangeJournalTracker(projectRoot)
	tracker.pending.Add(1)
	tracker.wg.Add(1)

	// Use the same cached storage provider as the rest of the process (daemon, CLI).
	// Creating a new FileObjectStorage per entry could use a spec loader that doesn't have
	// the builder registry set (e.g. in daemon child process), causing "builder registry not set"
	// validation failures. The cached provider was created with the registry set.
	baseCtx := parentCtx
	if baseCtx == nil {
		baseCtx = pkgctx.NewSystemContext()
	}
	bgCtx := context.WithoutCancel(baseCtx)
	goroutinelabels.NewGoroutine("storage-change-journal", "async createChangeJournalEntry").
		StartSimple(func() {
			defer func() {
				tracker.pending.Add(-1)
				tracker.wg.Done()
			}()
			ctx, cancel := context.WithTimeout(bgCtx, 10*time.Second)
			defer cancel()
			var storageProvider ObjectStorageProvider
			if fileStorage != nil {
				storageProvider = fileStorage
			} else {
				var err error
				storageProvider, err = GetGlobalStorageProviderCache().GetOrCreate(ctx, projectRoot)
				if err != nil {
					logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
					StorageLog(logger).Warn(LogEventStorageChangeJournalProviderCASRoutingWarn).
						WithError(err).
						Log()
					return
				}
			}
			err := CreateChangeJournalEntryWithBuilder(ctx, projectRoot, secCtx, storageProvider, options)
			if err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Warn(ConstMiscCreatechangejournalentrywithbuilderFaile).WithError(err).Log()
			}
		})

	return nil
}

// flattenUpdatePaths returns dotted paths for every updated key (including nested).
// Used for change journal analytics and for compaction (dictionary compression of path strings).
// Example: updates = {"meta": {"tags": ["a"], "version": 1}} => ["meta.tags", "meta.version"].
func flattenUpdatePaths(updates map[string]any) []string {
	if len(updates) == 0 {
		return nil
	}
	var paths []string
	flattenUpdatePathsRec(updates, "", &paths)
	sort.Strings(paths)
	return paths
}

func flattenUpdatePathsRec(m map[string]any, prefix string, out *[]string) {
	for k, v := range m {
		if k == "updated_at" || k == "updated_by" || k == ConstMiscExpectedUpdatedAt {
			continue
		}
		path := k
		if prefix != emptyValue {
			path = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			flattenUpdatePathsRec(val, path, out)
		default:
			*out = append(*out, path)
		}
	}
}

// buildDiffSummary creates a human-readable summary of what changed
func buildDiffSummary(updates, previousState map[string]any) string {
	if len(updates) == 0 {
		return "No changes"
	}

	var changes []string
	for field, newValue := range updates {
		// Skip internal/metadata fields
		if field == "updated_at" || field == "updated_by" || field == ConstMiscExpectedUpdatedAt {
			continue
		}

		var oldValue any
		if previousState != nil {
			oldValue = previousState[field]
		}

		if oldValue == nil {
			changes = append(changes, fmt.Sprintf(ConstMiscSAddedV, field, newValue))
		} else if fmt.Sprintf("%v", oldValue) != fmt.Sprintf("%v", newValue) {
			changes = append(changes, fmt.Sprintf("%s: %v -> %v", field, oldValue, newValue))
		}
	}

	if len(changes) == 0 {
		return ConstMiscMetadataUpdatedNoFieldChanges
	}

	return strings.Join(changes, "; ")
}
