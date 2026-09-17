package app

import (
	"context"
	"sync"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestValidationTriggerBatch_FlushOnSize(t *testing.T) {
	// Do not run in parallel: tests inject a global trigger func and would overwrite each other.
	var (
		mu    sync.Mutex
		calls []map[string]any
	)
	restore := SetValidationTriggerFuncForTest(func(_ context.Context, eventType, eventKind string, eventData map[string]any) error {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, eventData)
		return nil
	})
	defer restore()

	root := "test-batch-flush-on-size"
	for i := 0; i < validationBatchMaxSize; i++ {
		AddValidationTrigger(root, objects.KindBacklogItem, "BLI-"+string(rune('A'+i)), "create")
	}

	mu.Lock()
	n := len(calls)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("expected exactly 1 trigger call after %d adds, got %d", validationBatchMaxSize, n)
	}
	mu.Lock()
	payload := calls[0]
	mu.Unlock()
	items, ok := payload[validationEventKeyItems].([]any)
	if !ok {
		t.Fatalf("expected eventData[%q] []any, got %T", validationEventKeyItems, payload[validationEventKeyItems])
	}
	if len(items) != validationBatchMaxSize {
		t.Errorf("expected %d items in batch, got %d", validationBatchMaxSize, len(items))
	}
	for i, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			t.Errorf("item %d: expected map[string]any, got %T", i, it)
			continue
		}
		if k, _ := m[validationItemKeyKind].(string); k != objects.KindBacklogItem {
			t.Errorf("item %d: kind = %q, want %s", i, k, objects.KindBacklogItem)
		}
		if id, _ := m[validationItemKeyID].(string); id == EmptyValue {
			t.Errorf("item %d: id empty", i)
		}
		if op, _ := m[validationItemKeyOp].(string); op != "create" {
			t.Errorf("item %d: operation = %q, want create", i, op)
		}
	}
}

func TestValidationTriggerBatch_NoTriggerWhenEmptyProjectRoot(t *testing.T) {
	// Do not run in parallel: tests inject a global trigger func.
	var calls int
	restore := SetValidationTriggerFuncForTest(func(context.Context, string, string, map[string]any) error {
		calls++
		return nil
	})
	defer restore()

	AddValidationTrigger("", objects.KindBacklogItem, "BLI-1", "create")
	if calls != 0 {
		t.Errorf("expected no trigger call for empty project root, got %d", calls)
	}
}
