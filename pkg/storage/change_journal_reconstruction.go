package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ChangeJournalReconstructionService handles state reconstruction at specific timestamps
type ChangeJournalReconstructionService struct {
	storageProvider         ObjectStorageProvider
	reconstructionsRunTotal atomic.Int64
	entriesReversedTotal    atomic.Int64
}

// NewChangeJournalReconstructionService creates a new reconstruction service
func NewChangeJournalReconstructionService(storageProvider ObjectStorageProvider) *ChangeJournalReconstructionService {
	return &ChangeJournalReconstructionService{
		storageProvider: storageProvider,
	}
}

// GetReconstructionStats returns lifetime counters for reconstructions run and entries reversed.
func (s *ChangeJournalReconstructionService) GetReconstructionStats() (reconstructionsRun, entriesReversed int64) {
	return s.reconstructionsRunTotal.Load(), s.entriesReversedTotal.Load()
}

// ReconstructStateAtTimestamp reconstructs an object state and updates lifetime counters.
func (s *ChangeJournalReconstructionService) ReconstructStateAtTimestamp(
	ctx context.Context,
	currentObj map[string]any,
	objectID string,
	kind string,
	snapshotTimestamp time.Time,
	logger logging.Logger,
) (map[string]any, error) {
	s.reconstructionsRunTotal.Add(1)
	res, count, err := ReconstructStateAtTimestampWithCount(ctx, s.storageProvider, currentObj, objectID, kind, snapshotTimestamp, logger)
	if count > 0 {
		s.entriesReversedTotal.Add(count)
	}
	return res, err
}

// ReconstructStateAtTimestamp reconstructs an object's state at a specific timestamp
// by walking backwards through change journal entries and applying reverse changes.
//
// Algorithm:
// 1. Query change journal entries for the object (object_ref = "kind:objectID")
// 2. Filter entries where created_at > snapshotTimestamp (changes that happened AFTER snapshot)
// 3. Sort entries by created_at descending (most recent first)
// 4. Start with current object state
// 5. Walk backwards, applying reverse changes:
//   - For "update": restore previous_state (undo the update)
//   - For "delete": restore previous_state (object existed before deletion)
//   - For "create": object was created after snapshot, didn't exist at snapshot (return error)
//
// 6. After reversing all changes after snapshot, we have the state at snapshot timestamp
func ReconstructStateAtTimestamp(
	ctx context.Context,
	storageProvider ObjectStorageProvider,
	currentObj map[string]any,
	objectID string,
	kind string,
	snapshotTimestamp time.Time,
	logger logging.Logger,
) (map[string]any, error) {
	res, _, err := ReconstructStateAtTimestampWithCount(ctx, storageProvider, currentObj, objectID, kind, snapshotTimestamp, logger)
	return res, err
}

// ReconstructStateAtTimestampWithCount reconstructs an object's state at a specific timestamp
// and returns the reconstructed state along with the count of entries reversed.
func ReconstructStateAtTimestampWithCount(
	ctx context.Context,
	storageProvider ObjectStorageProvider,
	currentObj map[string]any,
	objectID string,
	kind string,
	snapshotTimestamp time.Time,
	logger logging.Logger,
) (map[string]any, int64, error) {
	secCtx, storageCtx := systemStorageContexts()
	objectRef := formatObjectRef(kind, objectID)

	// Query change journal entries for this object
	// Filter: object_ref = "kind:id" AND created_at > snapshotTimestamp
	// We need entries AFTER snapshot to reverse them
	// Sort by created_at descending
	filter := ListFilter{
		Kind: objects.KindChangeJournalEntry,
		Filters: map[string]any{
			objects.FieldKeyObjectRef: objectRef,
			objects.FieldKeyCreatedAt: map[string]any{
				"$gt": snapshotTimestamp.Format(time.RFC3339),
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: false, // Descending (most recent first)
	}

	result, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, 0, errfmt.Newf(ConstAuditFailedToQueryChangeJournalEntries).Wrap(err)
	}

	// If no entries found, object hasn't changed since snapshot timestamp
	// Use current state (it matches snapshot state)
	if len(result.Objects) == 0 {
		StorageLog(logger).Debug(LogEventStorageChangeJournalReconstructNoEntriesAfterSnapshotDebug).
			ObjectID(objectID).
			Log()
		return deepCopyObject(currentObj), 0, nil
	}

	// Sort entries by created_at descending (most recent first)
	// This ensures we process changes in reverse chronological order
	entries := result.Objects
	sort.Slice(entries, func(i, j int) bool {
		timeI := parseTimestamp(entries[i][objects.FieldKeyCreatedAt])
		timeJ := parseTimestamp(entries[j][objects.FieldKeyCreatedAt])
		return timeI.After(timeJ) // Descending order
	})

	// Start with current object state
	reconstructed := deepCopyObject(currentObj)
	var reversedCount int64

	// Walk backwards through entries, applying reverse changes
	// All entries are after snapshot timestamp (due to filter)
	for _, entry := range entries {
		entryTime := parseTimestamp(entry[objects.FieldKeyCreatedAt])

		// All entries should be after snapshot timestamp (due to filter)
		// But defensive check
		if !entryTime.After(snapshotTimestamp) {
			StorageLog(logger).Debug(LogEventStorageChangeJournalReconstructSkipBeforeSnapshotDebug).
				ObjectID(objectID).
				String("entry_time", entryTime.Format(time.RFC3339)).
				Log()
			continue
		}

		reversedCount++

		// Get change type
		changeType, _ := entry[objects.FieldKeyChangeType].(string)

		// Apply reverse change based on change type
		switch changeType {
		case OpUpdate:
			// For updates, restore previous_state
			if previousState, ok := entry[objects.FieldKeyPreviousState].(map[string]any); ok {
				reconstructed = previousState
				StorageLog(logger).Debug(LogEventStorageChangeJournalReconstructReverseUpdateDebug).
					ObjectID(objectID).
					String("entry_time", entryTime.Format(time.RFC3339)).
					Log()
			} else {
				StorageLog(logger).Warn(LogEventStorageChangeJournalReconstructMissingPrevStateUpdateWarn).
					ObjectID(objectID).
					String("entry_id", getString(entry, "id")).
					Log()
			}

		case OpDelete:
			// For deletes, restore previous_state (object existed before deletion)
			if previousState, ok := entry[objects.FieldKeyPreviousState].(map[string]any); ok {
				reconstructed = previousState
				StorageLog(logger).Debug(LogEventStorageChangeJournalReconstructReverseDeleteDebug).
					ObjectID(objectID).
					String("entry_time", entryTime.Format(time.RFC3339)).
					Log()
			} else {
				StorageLog(logger).Warn(LogEventStorageChangeJournalReconstructMissingPrevStateDeleteWarn).
					ObjectID(objectID).
					String("entry_id", getString(entry, "id")).
					Log()
			}

		case OpCreate:
			// If object was created after snapshot timestamp, it didn't exist at snapshot
			// This is an error condition - we can't reconstruct state for an object that didn't exist
			StorageLog(logger).Warn(LogEventStorageChangeJournalReconstructObjectCreatedAfterSnapshot).
				ObjectID(objectID).
				String("entry_time", entryTime.Format(time.RFC3339)).
				String(ConstAuditSnapshotTimestamp, snapshotTimestamp.Format(time.RFC3339)).
				Log()
			return nil, reversedCount, errfmt.Errorf(ConstAuditObjectWasCreatedAfterSnapshotTimestampCannotReconstructState,
				objectID, snapshotTimestamp.Format(time.RFC3339))

		case "import":
			// Imports are similar to creates - object was imported
			StorageLog(logger).Debug(LogEventStorageChangeJournalReconstructImportEntryDebug).
				ObjectID(objectID).
				String("entry_time", entryTime.Format(time.RFC3339)).
				Log()

		default:
			StorageLog(logger).Warn(LogEventStorageChangeJournalReconstructUnknownChangeTypeWarn).
				ObjectID(objectID).
				String("change_type", changeType).
				String("entry_id", getString(entry, "id")).
				Log()
		}

		// Continue processing all entries (they're all after snapshot timestamp)
		// We need to reverse all changes that happened after snapshot
	}

	StorageLog(logger).Info(LogEventStorageChangeJournalReconstructStateAtTimestampInfo).
		ObjectID(objectID).
		String(ConstAuditSnapshotTimestamp, snapshotTimestamp.Format(time.RFC3339Nano)).
		Int(ConstAuditEntriesProcessed, len(entries)).
		Log()

	return reconstructed, reversedCount, nil
}

// deepCopyObject creates a deep copy of an object map
func deepCopyObject(obj map[string]any) map[string]any {
	if obj == nil {
		return nil
	}

	result := make(map[string]any)
	for k, v := range obj {
		// Simple deep copy for now - handles maps and slices recursively
		switch val := v.(type) {
		case map[string]any:
			result[k] = deepCopyObject(val)
		case []any:
			result[k] = deepCopySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// deepCopySlice creates a deep copy of a slice
func deepCopySlice(slice []any) []any {
	if slice == nil {
		return nil
	}

	result := make([]any, len(slice))
	for i, v := range slice {
		switch val := v.(type) {
		case map[string]any:
			result[i] = deepCopyObject(val)
		case []any:
			result[i] = deepCopySlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// parseTimestamp parses a timestamp from various formats
func parseTimestamp(timestamp any) time.Time {
	if timestamp == nil {
		return time.Time{}
	}

	var timeStr string
	switch v := timestamp.(type) {
	case string:
		timeStr = v
	case time.Time:
		return v
	default:
		return time.Time{}
	}

	// Try RFC3339 first (with nanoseconds)
	if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
		return t
	}

	// Try RFC3339 (without nanoseconds)
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t
	}

	// Try ISO 8601 format (2006-01-02T15:04:05Z)
	if t, err := time.Parse(ConstAudit20060102t150405z, timeStr); err == nil {
		return t
	}

	// Try ISO 8601 with timezone
	if t, err := time.Parse(ConstAudit20060102t1504050700, timeStr); err == nil {
		return t
	}

	return time.Time{}
}

// getString safely extracts a string value from a map
func getString(m map[string]any, key string) string {
	if v := objects.GetString(m, key); v != "" {
		return v
	}
	return ""
}

// QueryChangeJournalEntries queries change journal entries for a specific object
// This is a helper function that can be used independently
func QueryChangeJournalEntries(
	ctx context.Context,
	storageProvider ObjectStorageProvider,
	objectID string,
	kind string,
	startTime, endTime time.Time,
) ([]map[string]any, error) {
	secCtx, storageCtx := systemStorageContexts()
	objectRef := formatObjectRef(kind, objectID)

	// Build filter
	filters := map[string]any{
		objects.FieldKeyObjectRef: objectRef,
	}

	// Add time range if specified
	if !startTime.IsZero() || !endTime.IsZero() {
		timeFilter := make(map[string]any)
		if !startTime.IsZero() {
			timeFilter["$gte"] = startTime.Format(time.RFC3339)
		}
		if !endTime.IsZero() {
			timeFilter["$lte"] = endTime.Format(time.RFC3339)
		}
		if len(timeFilter) > 0 {
			filters[objects.FieldKeyCreatedAt] = timeFilter
		}
	}

	filter := ListFilter{
		Kind:    objects.KindChangeJournalEntry,
		Filters: filters,
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: false, // Descending (most recent first)
	}

	result, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf(ConstAuditFailedToQueryChangeJournalEntries).Wrap(err)
	}

	return result.Objects, nil
}

// ExtractObjectRef extracts object ID and kind from an object_ref string
// Format: "kind:id"
func ExtractObjectRef(objectRef string) (kind, objectID string, err error) {
	parts := strings.Split(objectRef, ":")
	if len(parts) != 2 {
		return "", "", errfmt.Errorf(ConstAuditInvalidObjectRefFormatExpectedKindId, objectRef)
	}
	return parts[0], parts[1], nil
}

func systemStorageContexts() (*pkgctx.SecurityContext, *pkgctx.StorageContext) {
	return pkgctx.NewSystemSecurityContext(), pkgctx.GetStorageContext()
}

func formatObjectRef(kind, objectID string) string {
	return fmt.Sprintf("%s:%s", kind, objectID)
}
