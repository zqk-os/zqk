package storage

import (
	"context"
	"fmt"
	"sync/atomic"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

var (
	healthEntriesCreatedTotal atomic.Int64
	healthEntriesFailedTotal  atomic.Int64
)

// GetHealthCheckJournalStats returns lifetime counters for created and failed health check journal entries.
func GetHealthCheckJournalStats() (created, failed int64) {
	return healthEntriesCreatedTotal.Load(), healthEntriesFailedTotal.Load()
}

// HealthCheckChangeJournalPayload is the payload stored in previous_state for health_check journal entries.
// Enables aggregation and compaction (micro-GC) to process health results over time.
const changeJournalHealthObjectRefPrefix = "health_monitor:"

// CreateHealthCheckChangeJournalEntry appends a change_journal_entry for a health monitor run.
// change_type=health_check, object_ref=health_monitor:<monitorID>. Used to track health over time
// and compress at interval via existing change journal aggregation and compaction (micro-GC).
// Best-effort: returns nil on failure so monitor run is not blocked.
func CreateHealthCheckChangeJournalEntry(
	ctx context.Context,
	projectRoot string,
	storageProvider ObjectStorageProvider,
	monitorID string,
	status string,
	summary string,
	details map[string]any,
) error {
	if projectRoot == emptyValue || monitorID == emptyValue {
		return nil
	}
	objectRef := changeJournalHealthObjectRefPrefix + monitorID
	diffSummary := fmt.Sprintf("status=%s %s", status, summary)
	payload := map[string]any{
		objects.FieldKeyStatus:  status,
		objects.FieldKeySummary: summary,
	}
	if len(details) > 0 {
		payload["details"] = details
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	if fromCtx := pkgctx.GetSecurityContext(ctx); fromCtx != nil {
		secCtx = fromCtx
	}
	options := &ChangeJournalEntryOptions{
		ChangeType:    "health_check",
		ObjectRef:     objectRef,
		DiffSummary:   diffSummary,
		PreviousState: payload,
		Title:         ConstAuditHealthCheck + objectRef,
		CreatedBy:     "system",
	}
	err := CreateChangeJournalEntryWithBuilder(ctx, projectRoot, secCtx, storageProvider, options)
	if err != nil {
		healthEntriesFailedTotal.Add(1)
		return err
	}
	healthEntriesCreatedTotal.Add(1)
	return nil
}
