package economy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// cycle262MockStorage is a thread-safe storage provider for unit and edge testing.
type cycle262MockStorage struct {
	storage.ObjectStorageProvider
	mu            sync.Mutex
	objects       map[string]map[string]any
	readErr       error
	updateErr     error
	failReadIDs   map[string]error
	failUpdateIDs map[string]error
}

func newCycle262MockStorage() *cycle262MockStorage {
	return &cycle262MockStorage{
		objects:       make(map[string]map[string]any),
		failReadIDs:   make(map[string]error),
		failUpdateIDs: make(map[string]error),
	}
}

func (m *cycle262MockStorage) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err, ok := m.failReadIDs[id]; ok {
		return nil, err
	}
	if m.readErr != nil {
		return nil, m.readErr
	}

	obj, ok := m.objects[id]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}

	// Deep copy to prevent test race conditions
	copied := make(map[string]any, len(obj))
	for k, v := range obj {
		copied[k] = v
	}
	return copied, nil
}

func (m *cycle262MockStorage) Update(_ context.Context, _ *storage.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err, ok := m.failUpdateIDs[id]; ok {
		return err
	}
	if m.updateErr != nil {
		return m.updateErr
	}

	obj, ok := m.objects[id]
	if !ok {
		return storage.ErrObjectNotFound
	}

	for k, v := range updates {
		obj[k] = v
	}
	return nil
}

func (m *cycle262MockStorage) Shutdown(context.Context) error {
	return nil
}

// TestEconomicBroker_Initialization verifies constructor properties.
func TestEconomicBroker_Initialization(t *testing.T) {
	store := newCycle262MockStorage()
	broker := NewEconomicBroker(store)
	if broker == nil {
		t.Fatal("expected non-nil EconomicBroker")
	}
	if broker.store != store {
		t.Fatalf("expected store to be set, got %v", broker.store)
	}
}

// TestEconomicBroker_PolicyReadFailure exercises the policy lookup error path.
func TestEconomicBroker_PolicyReadFailure(t *testing.T) {
	ctx := context.Background()
	store := newCycle262MockStorage()
	store.failReadIDs["global_economic_policy"] = errors.New("simulated policy store failure")

	broker := NewEconomicBroker(store)
	err := broker.DeductQueryCredits(ctx, "partner-abc")
	if err == nil {
		t.Fatal("expected error when policy read fails, got nil")
	}
	if !strings.Contains(err.Error(), "no active economic policy found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestEconomicBroker_PartnerNotFound exercises the partner account lookup error path.
func TestEconomicBroker_PartnerNotFound(t *testing.T) {
	ctx := context.Background()
	store := newCycle262MockStorage()
	store.objects["global_economic_policy"] = map[string]any{
		objects.FieldKeyCostPerQuery:   2,
		objects.FieldKeyMaxCreditLimit: 50,
	}

	broker := NewEconomicBroker(store)
	err := broker.DeductQueryCredits(ctx, "non-existent-partner")
	if err == nil {
		t.Fatal("expected error when partner account is missing, got nil")
	}
	if !strings.Contains(err.Error(), "partner account non-existent-partner not found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestEconomicBroker_DefaultCostPerQuery verifies default cost of 1 when unspecified or 0.
func TestEconomicBroker_DefaultCostPerQuery(t *testing.T) {
	ctx := context.Background()

	t.Run("cost omitted from policy", func(t *testing.T) {
		store := newCycle262MockStorage()
		store.objects["global_economic_policy"] = map[string]any{
			objects.FieldKeyMaxCreditLimit: 10,
		}
		store.objects["partner-x"] = map[string]any{
			"credit_balance": 10,
		}

		broker := NewEconomicBroker(store)
		if err := broker.DeductQueryCredits(ctx, "partner-x"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bal := store.objects["partner-x"]["credit_balance"].(int); bal != 9 {
			t.Fatalf("expected balance 9 after default cost deduction, got %d", bal)
		}
	})

	t.Run("cost explicitly 0 in policy", func(t *testing.T) {
		store := newCycle262MockStorage()
		store.objects["global_economic_policy"] = map[string]any{
			objects.FieldKeyCostPerQuery:   0,
			objects.FieldKeyMaxCreditLimit: 10,
		}
		store.objects["partner-y"] = map[string]any{
			"credit_balance": 5,
		}

		broker := NewEconomicBroker(store)
		if err := broker.DeductQueryCredits(ctx, "partner-y"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bal := store.objects["partner-y"]["credit_balance"].(int); bal != 4 {
			t.Fatalf("expected balance 4 after default cost deduction, got %d", bal)
		}
	})
}

// TestEconomicBroker_CreditLimitBoundary tests exact boundary conditions for credit limits.
func TestEconomicBroker_CreditLimitBoundary(t *testing.T) {
	ctx := context.Background()

	store := newCycle262MockStorage()
	store.objects["global_economic_policy"] = map[string]any{
		objects.FieldKeyCostPerQuery:   5,
		objects.FieldKeyMaxCreditLimit: 10,
	}
	store.objects["partner-boundary"] = map[string]any{
		"credit_balance": 0,
	}

	broker := NewEconomicBroker(store)

	// Balance goes to -5: -5 >= -10 (allowed)
	if err := broker.DeductQueryCredits(ctx, "partner-boundary"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Balance goes to -10: -10 >= -10 (exact limit boundary, allowed)
	if err := broker.DeductQueryCredits(ctx, "partner-boundary"); err != nil {
		t.Fatalf("unexpected error at exact credit limit boundary: %v", err)
	}
	if bal := store.objects["partner-boundary"]["credit_balance"].(int); bal != -10 {
		t.Fatalf("expected balance -10, got %d", bal)
	}

	// Balance would become -15: -15 < -10 (rejected)
	err := broker.DeductQueryCredits(ctx, "partner-boundary")
	if err == nil {
		t.Fatal("expected credit limit rejection error, got nil")
	}
	if !strings.Contains(err.Error(), "insufficient credits: partner partner-boundary reached credit limit") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestEconomicBroker_StorageUpdateFailure exercises error propagation from Update.
func TestEconomicBroker_StorageUpdateFailure(t *testing.T) {
	ctx := context.Background()
	store := newCycle262MockStorage()
	store.objects["global_economic_policy"] = map[string]any{
		objects.FieldKeyCostPerQuery:   2,
		objects.FieldKeyMaxCreditLimit: 20,
	}
	store.objects["partner-fail-update"] = map[string]any{
		"credit_balance": 10,
	}
	store.failUpdateIDs["partner-fail-update"] = errors.New("disk write failure")

	broker := NewEconomicBroker(store)
	err := broker.DeductQueryCredits(ctx, "partner-fail-update")
	if err == nil {
		t.Fatal("expected update failure error, got nil")
	}
	if !strings.Contains(err.Error(), "disk write failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestEconomicBroker_ConcurrentDeductions tests ledger locking under concurrent calls.
func TestEconomicBroker_ConcurrentDeductions(t *testing.T) {
	ctx := context.Background()
	store := newCycle262MockStorage()
	store.objects["global_economic_policy"] = map[string]any{
		objects.FieldKeyCostPerQuery:   1,
		objects.FieldKeyMaxCreditLimit: 1000,
	}
	initialBalance := 200
	store.objects["partner-concur"] = map[string]any{
		"credit_balance": initialBalance,
	}

	broker := NewEconomicBroker(store)

	const goroutines = 20
	const iterations = 5
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("broker_concurrent_test", fmt.Sprintf("deduction worker %d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					if err := broker.DeductQueryCredits(ctx, "partner-concur"); err != nil {
						t.Errorf("deduction failed in worker %d: %v", workerID, err)
					}
				}
			})
	}

	wg.Wait()

	expectedBalance := initialBalance - (goroutines * iterations)
	actualBalance := store.objects["partner-concur"]["credit_balance"].(int)
	if actualBalance != expectedBalance {
		t.Fatalf("expected final balance %d, got %d", expectedBalance, actualBalance)
	}
}

// TestAgentMarketplace_Initialization verifies constructor properties.
func TestAgentMarketplace_Initialization(t *testing.T) {
	store := newCycle262MockStorage()
	broker := NewEconomicBroker(store)

	t.Run("with broker", func(t *testing.T) {
		m := NewAgentMarketplace(broker)
		if m == nil {
			t.Fatal("expected non-nil AgentMarketplace")
		}
		if m.skills == nil || m.leases == nil {
			t.Fatal("expected initialized skills and leases maps")
		}
		if m.broker != broker {
			t.Fatalf("expected broker to be configured, got %v", m.broker)
		}
	})

	t.Run("with nil broker", func(t *testing.T) {
		m := NewAgentMarketplace(nil)
		if m == nil {
			t.Fatal("expected non-nil AgentMarketplace")
		}
		if m.broker != nil {
			t.Fatal("expected nil broker")
		}
	})
}

// TestAgentMarketplace_BroadcastSkill_Validation exercises all validation failure paths.
func TestAgentMarketplace_BroadcastSkill_Validation(t *testing.T) {
	ctx := context.Background()
	m := NewAgentMarketplace(nil)

	t.Run("missing skill ID", func(t *testing.T) {
		ad := SkillAdvertisement{
			KernelID:    "kernel-1",
			CostPerUse:  1,
			Description: "missing skill id test",
			Available:   true,
		}
		err := m.BroadcastSkill(ctx, ad)
		if err == nil || !strings.Contains(err.Error(), "skill ID is required") {
			t.Fatalf("expected 'skill ID is required' error, got %v", err)
		}
	})

	t.Run("missing kernel ID", func(t *testing.T) {
		ad := SkillAdvertisement{
			SkillID:     "skill-test-1",
			KernelID:    "",
			CostPerUse:  1,
			Description: "missing kernel id test",
			Available:   true,
		}
		err := m.BroadcastSkill(ctx, ad)
		if err == nil || !strings.Contains(err.Error(), "kernel ID is required") {
			t.Fatalf("expected 'kernel ID is required' error, got %v", err)
		}
	})

	t.Run("valid advertisement and update", func(t *testing.T) {
		ad := SkillAdvertisement{
			SkillID:     "skill-valid",
			KernelID:    "kernel-prod-1",
			CostPerUse:  10,
			Description: "Original description",
			Available:   true,
		}
		if err := m.BroadcastSkill(ctx, ad); err != nil {
			t.Fatalf("unexpected error broadcasting valid skill: %v", err)
		}

		// Update advertisement
		ad.Description = "Updated description"
		ad.Available = false
		if err := m.BroadcastSkill(ctx, ad); err != nil {
			t.Fatalf("unexpected error updating skill: %v", err)
		}

		m.mu.RLock()
		storedAd := m.skills["skill-valid"]
		m.mu.RUnlock()

		if storedAd.Description != "Updated description" || storedAd.Available != false {
			t.Fatalf("skill was not updated properly: %+v", storedAd)
		}
	})
}

// TestAgentMarketplace_GetAvailableSkills_Filters verifies filtering of available skills.
func TestAgentMarketplace_GetAvailableSkills_Filters(t *testing.T) {
	ctx := context.Background()
	m := NewAgentMarketplace(nil)

	// Initially empty
	skills, err := m.GetAvailableSkills(ctx)
	if err != nil {
		t.Fatalf("unexpected error on empty marketplace: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(skills))
	}

	// Add unavailable skills only
	_ = m.BroadcastSkill(ctx, SkillAdvertisement{
		SkillID:   "off-1",
		KernelID:  "node-1",
		Available: false,
	})
	_ = m.BroadcastSkill(ctx, SkillAdvertisement{
		SkillID:   "off-2",
		KernelID:  "node-2",
		Available: false,
	})

	skills, err = m.GetAvailableSkills(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected 0 available skills, got %d", len(skills))
	}

	// Add an available skill
	_ = m.BroadcastSkill(ctx, SkillAdvertisement{
		SkillID:   "on-1",
		KernelID:  "node-3",
		Available: true,
	})

	skills, err = m.GetAvailableSkills(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 || skills[0].SkillID != "on-1" {
		t.Fatalf("expected [on-1], got %+v", skills)
	}
}

// TestAgentMarketplace_LeaseSkill_Errors exercises all failure conditions for LeaseSkill.
func TestAgentMarketplace_LeaseSkill_Errors(t *testing.T) {
	ctx := context.Background()

	store := newCycle262MockStorage()
	store.objects["global_economic_policy"] = map[string]any{
		objects.FieldKeyCostPerQuery:   10,
		objects.FieldKeyMaxCreditLimit: 0,
	}
	store.objects["broke-consumer"] = map[string]any{
		"credit_balance": 0,
	}

	broker := NewEconomicBroker(store)
	m := NewAgentMarketplace(broker)

	_ = m.BroadcastSkill(ctx, SkillAdvertisement{
		SkillID:   "skill-active",
		KernelID:  "provider-node",
		Available: true,
	})
	_ = m.BroadcastSkill(ctx, SkillAdvertisement{
		SkillID:   "skill-disabled",
		KernelID:  "provider-node",
		Available: false,
	})

	t.Run("skill does not exist", func(t *testing.T) {
		_, err := m.LeaseSkill(ctx, "broke-consumer", "skill-unknown", time.Minute)
		if err == nil || !strings.Contains(err.Error(), "skill skill-unknown not found") {
			t.Fatalf("expected 'skill not found' error, got %v", err)
		}
	})

	t.Run("skill is not available", func(t *testing.T) {
		_, err := m.LeaseSkill(ctx, "broke-consumer", "skill-disabled", time.Minute)
		if err == nil || !strings.Contains(err.Error(), "skill skill-disabled is not currently available") {
			t.Fatalf("expected 'is not currently available' error, got %v", err)
		}
	})

	t.Run("credit check deduction failure", func(t *testing.T) {
		_, err := m.LeaseSkill(ctx, "broke-consumer", "skill-active", time.Minute)
		if err == nil || !strings.Contains(err.Error(), "failed to lease skill, credit check failed") {
			t.Fatalf("expected credit check failed error, got %v", err)
		}
	})
}

// TestAgentMarketplace_LeaseSkill_NilBroker verifies leasing succeeds when no broker is attached.
func TestAgentMarketplace_LeaseSkill_NilBroker(t *testing.T) {
	ctx := context.Background()
	m := NewAgentMarketplace(nil)

	ad := SkillAdvertisement{
		SkillID:     "free-skill",
		KernelID:    "host-alpha",
		CostPerUse:  0,
		Description: "Community free skill",
		Available:   true,
	}
	if err := m.BroadcastSkill(ctx, ad); err != nil {
		t.Fatalf("unexpected broadcast error: %v", err)
	}

	leaseDuration := 30 * time.Minute
	lease, err := m.LeaseSkill(ctx, "client-node", "free-skill", leaseDuration)
	if err != nil {
		t.Fatalf("unexpected lease error with nil broker: %v", err)
	}

	if lease.SkillID != "free-skill" {
		t.Fatalf("expected SkillID 'free-skill', got %s", lease.SkillID)
	}
	if lease.ProviderID != "host-alpha" {
		t.Fatalf("expected ProviderID 'host-alpha', got %s", lease.ProviderID)
	}
	if lease.ConsumerID != "client-node" {
		t.Fatalf("expected ConsumerID 'client-node', got %s", lease.ConsumerID)
	}
	if !strings.HasPrefix(lease.LeaseID, "lease-client-node-free-skill-") {
		t.Fatalf("expected lease ID prefix 'lease-client-node-free-skill-', got %s", lease.LeaseID)
	}
	if lease.ExpiresAt.Before(time.Now()) {
		t.Fatalf("expected future expiration, got %v", lease.ExpiresAt)
	}

	// Verify lease is saved in internal leases map
	m.mu.RLock()
	savedLease, exists := m.leases[lease.LeaseID]
	m.mu.RUnlock()
	if !exists || savedLease.LeaseID != lease.LeaseID {
		t.Fatalf("expected lease to be stored in marketplace leases map: %+v", savedLease)
	}
}

// TestAgentMarketplace_ConcurrentOperations tests concurrent broadcasts, queries, and leases.
func TestAgentMarketplace_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()

	store := newCycle262MockStorage()
	store.objects["global_economic_policy"] = map[string]any{
		objects.FieldKeyCostPerQuery:   1,
		objects.FieldKeyMaxCreditLimit: 5000,
	}
	for i := 0; i < 10; i++ {
		consumerID := fmt.Sprintf("concurrent-consumer-%d", i)
		store.objects[consumerID] = map[string]any{
			"credit_balance": 1000,
		}
	}

	broker := NewEconomicBroker(store)
	m := NewAgentMarketplace(broker)

	// Seed an initial skill
	_ = m.BroadcastSkill(ctx, SkillAdvertisement{
		SkillID:   "seed-skill",
		KernelID:  "seed-provider",
		Available: true,
	})

	const numWorkers = 8
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		workerIdx := i
		goroutinelabels.NewGoroutine("marketplace_concurrency", fmt.Sprintf("worker %d", workerIdx)).
			StartSimple(func() {
				defer wg.Done()

				skillID := fmt.Sprintf("skill-worker-%d", workerIdx)
				consumerID := fmt.Sprintf("concurrent-consumer-%d", workerIdx)

				// 1. Broadcast skill
				_ = m.BroadcastSkill(ctx, SkillAdvertisement{
					SkillID:   skillID,
					KernelID:  fmt.Sprintf("provider-%d", workerIdx),
					Available: true,
				})

				// 2. Query available skills
				_, _ = m.GetAvailableSkills(ctx)

				// 3. Lease seed skill
				_, _ = m.LeaseSkill(ctx, consumerID, "seed-skill", time.Minute)

				// 4. Lease own skill
				_, _ = m.LeaseSkill(ctx, consumerID, skillID, time.Minute)
			})
	}

	wg.Wait()

	skills, err := m.GetAvailableSkills(ctx)
	if err != nil {
		t.Fatalf("failed to get available skills after concurrency test: %v", err)
	}
	if len(skills) < numWorkers {
		t.Fatalf("expected at least %d skills registered, got %d", numWorkers, len(skills))
	}
}
