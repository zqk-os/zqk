// Traceability: core-backlog, core-backlog
package economy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SkillAdvertisement represents an AI skill offered by a kernel on the marketplace.
type SkillAdvertisement struct {
	SkillID     string
	KernelID    string
	CostPerUse  int
	Description string
	Available   bool
}

// SkillLease represents an active lease of an AI skill by a consumer kernel.
type SkillLease struct {
	LeaseID    string
	SkillID    string
	ProviderID string
	ConsumerID string
	ExpiresAt  time.Time
}

// AgentMarketplace allows kernels to broadcast available AI skills and accept lease requests.
type AgentMarketplace struct {
	mu     sync.RWMutex
	skills map[string]SkillAdvertisement
	leases map[string]SkillLease
	broker *EconomicBroker
}

// NewAgentMarketplace creates a new AgentMarketplace instance.
func NewAgentMarketplace(broker *EconomicBroker) *AgentMarketplace {
	return &AgentMarketplace{
		skills: make(map[string]SkillAdvertisement),
		leases: make(map[string]SkillLease),
		broker: broker,
	}
}

// BroadcastSkill advertises a skill to the marketplace.
func (m *AgentMarketplace) BroadcastSkill(ctx context.Context, ad SkillAdvertisement) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ad.SkillID == "" {
		return fmt.Errorf("skill ID is required")
	}
	if ad.KernelID == "" {
		return fmt.Errorf("kernel ID is required")
	}

	m.skills[ad.SkillID] = ad
	return nil
}

// GetAvailableSkills returns a list of all currently available skills.
func (m *AgentMarketplace) GetAvailableSkills(ctx context.Context) ([]SkillAdvertisement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var available []SkillAdvertisement
	for _, ad := range m.skills {
		if ad.Available {
			available = append(available, ad)
		}
	}
	return available, nil
}

// LeaseSkill attempts to lease a skill for a consumer kernel.
func (m *AgentMarketplace) LeaseSkill(ctx context.Context, consumerID, skillID string, duration time.Duration) (SkillLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ad, exists := m.skills[skillID]
	if !exists {
		return SkillLease{}, fmt.Errorf("skill %s not found", skillID)
	}
	if !ad.Available {
		return SkillLease{}, fmt.Errorf("skill %s is not currently available", skillID)
	}

	if m.broker != nil {
		if err := m.broker.DeductQueryCredits(ctx, consumerID); err != nil {
			return SkillLease{}, fmt.Errorf("failed to lease skill, credit check failed: %w", err)
		}
	}

	leaseID := fmt.Sprintf("lease-%s-%s-%d", consumerID, skillID, time.Now().UnixNano())
	lease := SkillLease{
		LeaseID:    leaseID,
		SkillID:    skillID,
		ProviderID: ad.KernelID,
		ConsumerID: consumerID,
		ExpiresAt:  time.Now().Add(duration),
	}

	m.leases[leaseID] = lease

	return lease, nil
}
