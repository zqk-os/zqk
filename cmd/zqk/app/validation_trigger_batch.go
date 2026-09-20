// Package app: batches object validation triggers and flushes by size or time
// so the reusable SCH-val job runs once per batch instead of once per change.

package app

import (
	"context"
	"sync"
	"time"

	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

const (
	validationBatchMaxSize = 20              // flush when this many items queued per project
	validationBatchMaxWait = 3 * time.Second // flush after this long since first item
)

const (
	validationEventKindBatch = "batch"
	validationEventKeyItems  = "items"
	validationItemKeyKind    = "kind"
	validationItemKeyID      = "id"
	validationItemKeyOp      = "operation"
)

type validationItem struct {
	Kind      string
	ID        string
	Operation string
}

type projectBatch struct {
	mu          sync.Mutex
	items       []validationItem
	timer       *time.Timer
	projectRoot string
}

var (
	validationBatchMu     sync.Mutex
	validationBatchByRoot = make(map[string]*projectBatch)
	// validationTriggerFunc, when set (e.g. in tests), is used instead of the global scheduler for flushing.
	// Allows unit tests to verify batch size and payload without a real scheduler.
	validationTriggerFunc func(ctx context.Context, eventType, eventKind string, eventData map[string]any) error
)

// SetValidationTriggerFuncForTest injects a trigger function for tests. Returns a restore func to clear it.
// Production code leaves this nil and uses GetGlobalScheduler().TriggerJobByEvent.
func SetValidationTriggerFuncForTest(f func(context.Context, string, string, map[string]any) error) (restore func()) {
	validationTriggerFunc = f
	return func() { validationTriggerFunc = nil }
}

// AddValidationTrigger adds a single (kind, id, operation) to the batch for projectRoot.
// Flushes when the buffer reaches validationBatchMaxSize or after validationBatchMaxWait
// since the first item in the buffer. No-op if projectRoot is empty or no scheduler/trigger available.
func AddValidationTrigger(projectRoot string, kind, id, operation string) {
	if projectRoot == EmptyValue {
		return
	}
	if schedulerpkg.ShouldSkipBackgroundValidationBatchForKind(kind) {
		return
	}
	if schedulerpkg.GetGlobalScheduler() == nil && validationTriggerFunc == nil {
		return
	}

	validationBatchMu.Lock()
	pb, ok := validationBatchByRoot[projectRoot]
	if !ok {
		pb = &projectBatch{projectRoot: projectRoot}
		validationBatchByRoot[projectRoot] = pb
	}
	validationBatchMu.Unlock()

	pb.mu.Lock()
	pb.items = append(pb.items, validationItem{Kind: kind, ID: id, Operation: operation})
	shouldFlush := len(pb.items) >= validationBatchMaxSize
	if len(pb.items) == 1 {
		pb.timer = time.AfterFunc(validationBatchMaxWait, func() {
			flushValidationBatch(projectRoot)
		})
	}
	pb.mu.Unlock()

	if shouldFlush {
		flushValidationBatch(projectRoot)
	}
}

// flushValidationBatch takes the current buffer for projectRoot, clears it, stops the timer,
// and triggers the object_validation job once with eventData {"items": [...]}.
func flushValidationBatch(projectRoot string) {
	validationBatchMu.Lock()
	pb := validationBatchByRoot[projectRoot]
	validationBatchMu.Unlock()
	if pb == nil {
		return
	}

	pb.mu.Lock()
	items := pb.items
	pb.items = nil
	if pb.timer != nil {
		pb.timer.Stop()
		pb.timer = nil
	}
	pb.mu.Unlock()

	if len(items) == 0 {
		return
	}

	eventItems := make([]any, len(items))
	for i := range items {
		eventItems[i] = map[string]any{
			validationItemKeyKind: items[i].Kind,
			validationItemKeyID:   items[i].ID,
			validationItemKeyOp:   items[i].Operation,
		}
	}
	eventData := map[string]any{validationEventKeyItems: eventItems}
	ctx := context.Background() // Background: request-or-shutdown derived
	if validationTriggerFunc != nil {
		_ = validationTriggerFunc(ctx, schedulerpkg.ObjectValidationEventType, validationEventKindBatch, eventData)
		return
	}
	sched := schedulerpkg.GetGlobalScheduler()
	if sched != nil {
		_ = sched.TriggerJobByEvent(ctx, schedulerpkg.ObjectValidationEventType, validationEventKindBatch, eventData)
	}
}
