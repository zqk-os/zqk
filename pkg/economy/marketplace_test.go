// Traceability: ITEM-EXAMPLE, REQ-REDACTED
package economy_test

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/economy"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestAgentMarketplace_BroadcastAndGetSkills(t *testing.T) {
	ctx := context.Background()
	marketplace := economy.NewAgentMarketplace(nil)

	ad := economy.SkillAdvertisement{
		SkillID:     "skill-1",
		KernelID:    "provider-node",
		CostPerUse:  5,
		Description: "A cool skill",
		Available:   true,
	}

	if err := marketplace.BroadcastSkill(ctx, ad); err != nil {
		t.Fatalf("failed to broadcast skill: %v", err)
	}

	// Invalid broadcast
	invalidAd := economy.SkillAdvertisement{
		KernelID: "provider-node", // missing SkillID
	}
	if err := marketplace.BroadcastSkill(ctx, invalidAd); err == nil {
		t.Errorf("expected error when broadcasting skill without ID")
	}

	skills, err := marketplace.GetAvailableSkills(ctx)
	if err != nil {
		t.Fatalf("failed to get skills: %v", err)
	}

	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].SkillID != "skill-1" {
		t.Errorf("expected skill-1, got %s", skills[0].SkillID)
	}
}

func TestAgentMarketplace_LeaseSkill(t *testing.T) {
	ctx := context.Background()

	store := &mockStorage{
		objects: map[string]map[string]any{
			"global_economic_policy": {
				objects.FieldKeyCostPerQuery:   5,
				objects.FieldKeyMaxCreditLimit: 10,
			},
			"consumer-node": {
				"credit_balance": 10,
			},
		},
	}

	broker := economy.NewEconomicBroker(store)
	marketplace := economy.NewAgentMarketplace(broker)

	ad := economy.SkillAdvertisement{
		SkillID:     "skill-2",
		KernelID:    "provider-node",
		CostPerUse:  5,
		Description: "Another cool skill",
		Available:   true,
	}

	_ = marketplace.BroadcastSkill(ctx, ad)

	// Lease successful
	lease, err := marketplace.LeaseSkill(ctx, "consumer-node", "skill-2", time.Hour)
	if err != nil {
		t.Fatalf("failed to lease skill: %v", err)
	}

	if lease.SkillID != "skill-2" || lease.ConsumerID != "consumer-node" || lease.ProviderID != "provider-node" {
		t.Errorf("invalid lease details: %+v", lease)
	}

	// Verify credits were deducted
	if bal := store.objects["consumer-node"]["credit_balance"].(int); bal != 5 {
		t.Errorf("expected balance 5, got %d", bal)
	}

	// Lease fail: non-existent skill
	if _, err := marketplace.LeaseSkill(ctx, "consumer-node", "skill-99", time.Hour); err == nil {
		t.Errorf("expected error when leasing non-existent skill")
	}

	// Add unavailable skill
	unavailAd := economy.SkillAdvertisement{
		SkillID:   "skill-3",
		KernelID:  "provider-node",
		Available: false,
	}
	_ = marketplace.BroadcastSkill(ctx, unavailAd)

	if _, err := marketplace.LeaseSkill(ctx, "consumer-node", "skill-3", time.Hour); err == nil {
		t.Errorf("expected error when leasing unavailable skill")
	}
}
