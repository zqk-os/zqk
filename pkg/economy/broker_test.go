package economy_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/economy"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockStorage struct {
	storage.ObjectStorageProvider
	objects map[string]map[string]any
}

func (m *mockStorage) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, storage.ErrObjectNotFound
}

func (m *mockStorage) Update(_ context.Context, _ *storage.SecurityContext, id string, updates map[string]any) error {
	if obj, ok := m.objects[id]; ok {
		for k, v := range updates {
			obj[k] = v
		}
		return nil
	}
	return storage.ErrObjectNotFound
}

func TestEconomicBroker_DeductQueryCredits(t *testing.T) {
	ctx := context.Background()
	store := &mockStorage{
		objects: map[string]map[string]any{
			"global_economic_policy": {
				objects.FieldKeyCostPerQuery:   5,
				objects.FieldKeyMaxCreditLimit: 10,
			},
			"partner-001": {
				"credit_balance": 10,
			},
		},
	}

	broker := economy.NewEconomicBroker(store)

	// 1. First deduction (Success)
	if err := broker.DeductQueryCredits(ctx, "partner-001"); err != nil {
		t.Fatalf("first deduction failed: %v", err)
	}

	if bal := store.objects["partner-001"]["credit_balance"].(int); bal != 5 {
		t.Errorf("expected balance 5, got %d", bal)
	}

	// 2. Second deduction (Success, hitting zero)
	_ = broker.DeductQueryCredits(ctx, "partner-001")
	if bal := store.objects["partner-001"]["credit_balance"].(int); bal != 0 {
		t.Errorf("expected balance 0, got %d", bal)
	}

	// 3. Third deduction (Success, into debt but within limit)
	_ = broker.DeductQueryCredits(ctx, "partner-001") // -5
	_ = broker.DeductQueryCredits(ctx, "partner-001") // -10

	// 4. Fourth deduction (Fail, exceeds limit)
	if err := broker.DeductQueryCredits(ctx, "partner-001"); err == nil {
		t.Errorf("expected error due to credit limit, got nil")
	}
}

func (m *mockStorage) Shutdown(context.Context) error { return nil }
