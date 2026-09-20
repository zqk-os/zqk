package storage

import (
	"context"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ChangeJournalCompactionService handles compaction of change journal entries
type ChangeJournalCompactionService struct {
	storage               ObjectStorageProvider
	compactionsRunTotal   atomic.Int64
	entriesCompactedTotal atomic.Int64
}

// NewChangeJournalCompactionService creates a new change journal compaction service
func NewChangeJournalCompactionService(storage ObjectStorageProvider) *ChangeJournalCompactionService {
	return &ChangeJournalCompactionService{
		storage: storage,
	}
}

// GetCompactionStats returns lifetime counters for compactions run and entries compacted.
func (s *ChangeJournalCompactionService) GetCompactionStats() (compactionsRun, entriesCompacted int64) {
	return s.compactionsRunTotal.Load(), s.entriesCompactedTotal.Load()
}

// CompactWindow compacts change journal entries within a time window
// 1. Queries entries in the window
// 2. Compacts them into a single artifact using dictionary compression
// 3. Writes the artifact to the output directory
// 4. Deletes the original entries
func (s *ChangeJournalCompactionService) CompactWindow(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	windowStart, windowEnd time.Time,
	outputDir string,
) (*CompactionResult, error) {
	// Use system logger
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	StorageLog(logger).Info(LogEventStorageChangeJournalCompactionStartingInfo).
		String("start", windowStart.Format(time.RFC3339)).
		String("end", windowEnd.Format(time.RFC3339)).
		Log()

	// 1. Query entries
	filter := ListFilter{
		Kind: objects.KindChangeJournalEntry,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$gte": windowStart.Format(time.RFC3339),
				"$lte": windowEnd.Format(time.RFC3339),
			},
		},
		SortBy:  "created_at",
		SortAsc: true,
		Limit:   0, // No limit - get all entries in window
	}

	result, err := s.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf(ConstAuditFailedToListChangeJournalEntries).Wrap(err)
	}

	entries := result.Objects
	if len(entries) == 0 {
		StorageLog(logger).Info(LogEventStorageChangeJournalCompactionNoEntriesInfo).Log()
		return &CompactionResult{
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
			EntryCount:  0,
		}, nil
	}

	StorageLog(logger).Info(LogEventStorageChangeJournalCompactionFoundEntriesInfo).Count(len(entries)).Log()

	// 2. Compact and write artifact
	compactionResult, err := CompactChangeJournalWindow(entries, outputDir, windowStart, windowEnd, logger)
	if err != nil {
		return nil, errfmt.Newf(ConstAuditFailedToCompactWindow).Wrap(err)
	}

	s.compactionsRunTotal.Add(1)
	s.entriesCompactedTotal.Add(int64(len(entries)))

	StorageLog(logger).Info(LogEventStorageChangeJournalCompactionCompactedInfo).
		Path(compactionResult.ArtifactPath).
		Int(ConstAuditDictionarySize, compactionResult.DictionarySize).
		Log()

	// 3. Delete original entries
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if id := objects.GetString(e, objects.FieldKeyID); id != "" {
			ids = append(ids, id)
		}
	}

	if len(ids) > 0 {
		StorageLog(logger).Info(LogEventStorageChangeJournalCompactionDeletingOrigInfo).Count(len(ids)).Log()

		// Authorize bulk delete by marking as CLI operation (same package helper)
		deleteCtx := WithCLIOperation(ctx)

		// Use BulkDelete for efficiency
		// We can set cascade=false as change journal entries don't usually have dependents that need cascading
		bulkResult, err := s.storage.BulkDelete(deleteCtx, secCtx, ids, false)
		if err != nil {
			// Even if delete fails, we return the result because compaction succeeded
			// But we log the error
			StorageLog(logger).Error(LogEventStorageChangeJournalCompactionDeleteOrigFailed, err).Log()
			return compactionResult, errfmt.Newf(ConstAuditCompactionSucceededButFailedToDeleteOriginals).Wrap(err)
		}

		if bulkResult.FailureCount > 0 {
			StorageLog(logger).Warn(LogEventStorageChangeJournalCompactionSomeDeletesFailedWarn).
				Int("success", bulkResult.SuccessCount).
				Int("failure", bulkResult.FailureCount).
				Log()
		}
	}

	return compactionResult, nil
}
