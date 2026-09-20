package audit

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// FlushPlan is the allocated aggregated_summary plus id. I/O stays in storage.
type FlushPlan struct {
	ID    string
	Event map[string]any
	Now   time.Time
}

// FlushActor is created_by for a buffer flush. Empty account → "system"
// (not pkgctx.SystemAccountID) to match the historical flush payload.
func FlushActor(accountID string) string {
	if accountID == "" {
		return ActorSystem
	}
	return accountID
}

// FlushActorFromSec returns FlushActor for a security context.
func FlushActorFromSec(secCtx *pkgctx.SecurityContext) string {
	if secCtx == nil {
		return ActorSystem
	}
	return FlushActor(secCtx.AccountID)
}

// PrepareFlush allocates an id and builds the aggregated_summary map.
func PrepareFlush(ids IDAllocator, accountID, key string, group *Group, now time.Time) (FlushPlan, error) {
	var plan FlushPlan
	if ids == nil || group == nil {
		return plan, nil
	}
	id, err := ids.GenerateNextID()
	if err != nil {
		return plan, err
	}
	plan.ID = id
	plan.Now = now
	plan.Event = AggregatedSummaryMap(id, key, FlushActor(accountID), group, now)
	return plan, nil
}

// FlushMetricTags are TSDB tags for an aggregated_summary write.
func FlushMetricTags(group *Group) map[string]string {
	if group == nil {
		return map[string]string{}
	}
	return map[string]string{
		objects.FieldKeyEventType:  group.EventType,
		objects.FieldKeyTargetKind: group.TargetKind,
		objects.FieldKeySeverity:   group.Severity,
	}
}

// SummaryValidation is the best-effort validate outcome for a flush payload.
type SummaryValidation struct {
	OK       bool
	Err      error
	Messages []string
}

// ValidateSummary runs lifecycle validation on an aggregated_summary map.
func ValidateSummary(ctx context.Context, event map[string]any) SummaryValidation {
	var out SummaryValidation
	if event == nil {
		return out
	}
	result, err := validation.NewGoValidator().Validate(ctx, event, Kind, &validation.ValidationOptions{
		CurrentState:          StatusCompleted,
		ValidateLifecycle:     true,
		ValidateSemanticTypes: false,
	})
	if err != nil {
		out.Err = err
		return out
	}
	if result == nil {
		return out
	}
	out.OK = result.IsValid
	if result.IsValid {
		return out
	}
	out.Messages = make([]string, 0, len(result.Errors))
	for _, e := range result.Errors {
		out.Messages = append(out.Messages, fmt.Sprintf("%s: %s", e.Field, e.Message))
	}
	return out
}

// RecordFlushWrite records flush persist metrics. Validation is independent of write success.
func RecordFlushWrite(ctx context.Context, rec CreationRecorder, usedCAS bool, duration time.Duration, writeOK, validOK bool) {
	if rec == nil {
		return
	}
	rec.RecordAuditEventCreation(ctx, EventTypeAggregatedSummary, usedCAS, duration, writeOK)
	rec.RecordAuditEventValidation(validOK)
}

// AllocatorFactory builds an IDAllocator for a monthly audit dir.
type AllocatorFactory func(ctx context.Context, dir string) IDAllocator

// FlushRun allocates an id, validates the aggregated_summary, then writes via
// FlushSink. Mkdir stays in storage. Flush must not use EventStore.Create.
func FlushRun(
	allocCtx, validateCtx context.Context,
	dir string,
	newIDs AllocatorFactory,
	sink FlushSink,
	rec CreationRecorder,
	accountID, key string,
	group *Group,
	now time.Time,
) (plan FlushPlan, validated SummaryValidation, written FlushWriteResult, err error) {
	var ids IDAllocator
	if newIDs != nil {
		ids = newIDs(allocCtx, dir)
	}
	plan, err = PrepareFlush(ids, accountID, key, group, now)
	if err != nil {
		return plan, validated, written, err
	}
	validated = ValidateSummary(validateCtx, plan.Event)
	written = WriteFlush(validateCtx, sink, rec, dir, plan.ID, plan.Event, FlushMetricTags(group), validated.OK)
	return plan, validated, written, nil
}
