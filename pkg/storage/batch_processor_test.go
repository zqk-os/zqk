package storage

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestBatchProcessor_LifetimeCounters(t *testing.T) {
	bp := NewBatchProcessor(2)

	batches, items := bp.GetBatchStats()
	if batches != 0 || items != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", batches, items)
	}

	queryFunc := func(ctx context.Context, offset, limit int) ([]map[string]any, error) {
		if offset >= 3 {
			return nil, nil
		}
		if offset == 0 {
			return []map[string]any{{objects.FieldKeyID: "1"}, {objects.FieldKeyID: "2"}}, nil
		}
		return []map[string]any{{objects.FieldKeyID: "3"}}, nil
	}

	processFunc := func(batch []map[string]any) (any, []string, error) {
		ids := make([]string, len(batch))
		for i, item := range batch {
			ids[i] = item[objects.FieldKeyID].(string)
		}
		return len(batch), ids, nil
	}

	mergeFunc := func(firstResult, secondResult any) (any, error) {
		return firstResult.(int) + secondResult.(int), nil
	}

	ctx := context.Background()
	result, ids, err := bp.ProcessInBatches(ctx, queryFunc, processFunc, mergeFunc)
	if err != nil {
		t.Fatalf("ProcessInBatches error = %v", err)
	}
	if result.(int) != 3 {
		t.Errorf("expected result 3, got %v", result)
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 ids, got %d", len(ids))
	}

	batches, items = bp.GetBatchStats()
	if batches != 2 || items != 3 {
		t.Errorf("expected batches=2 items=3, got batches=%d items=%d", batches, items)
	}
}
