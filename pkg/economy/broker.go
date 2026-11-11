package economy

import (
	"context"
	"fmt"
	"sync"

	"github.com/zqk-os/zqk/pkg/logging"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// EconomicBroker enforces economic policies on kernel-to-kernel interactions.
type EconomicBroker struct {
	store storage.ObjectStorageProvider
	mu    sync.Mutex
}

// NewEconomicBroker creates a new EconomicBroker.
func NewEconomicBroker(store storage.ObjectStorageProvider) *EconomicBroker {
	return &EconomicBroker{store: store}
}

// DeductQueryCredits deducts credits from a partner kernel based on the active policy.
func (b *EconomicBroker) DeductQueryCredits(ctx context.Context, partnerID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 1. Load active policy (simplified: just read 'global_economic_policy')
	policy, err := b.store.Read(ctx, nil, "global_economic_policy")
	if err != nil {
		return fmt.Errorf("no active economic policy found")
	}

	cost, _ := policy[objects.FieldKeyCostPerQuery].(int)
	if cost == 0 {
		cost = 1 // Default cost
	}

	// 2. Load partner account
	partner, err := b.store.Read(ctx, nil, partnerID)
	if err != nil {
		return fmt.Errorf("partner account %s not found", partnerID)
	}

	balance, _ := partner["credit_balance"].(int)
	limit, _ := policy[objects.FieldKeyMaxCreditLimit].(int)

	newBalance := balance - cost

	// 3. Enforce credit limit
	if newBalance < -limit {
		return fmt.Errorf("insufficient credits: partner %s reached credit limit", partnerID)
	}

	// 4. Update partner account
	updates := map[string]any{
		"credit_balance": newBalance,
	}

	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [economy] market-broker: Deducted %d credits from %s (Remaining: %d)\n", cost, partnerID, newBalance)).Log()

	return b.store.Update(ctx, nil, partnerID, updates)
}
