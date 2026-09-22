package cef

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvidenceLift_IntermediateLevel(t *testing.T) {
	// Case where all dimensions are low (< targetThreshold 3.6 after lift)
	low := EvidenceTier{
		Test:     1.0,
		Receive:  2.0,
		Security: 1.5,
	}

	lifted := low.LiftEvidence()
	assert.Equal(t, EnvelopeFloor4, lifted.CurrentLevel, "should fall back to intermediate level EnvelopeFloor4")

	// Case where Security is higher than Test and Receive
	lowSec := EvidenceTier{
		Test:     1.0,
		Receive:  1.2,
		Security: 2.2,
	}
	liftedSec := lowSec.LiftEvidence()
	assert.Equal(t, EnvelopeFloor4, liftedSec.CurrentLevel)

	// Direct call to intermediateLevel with safeMax >= EnvelopeFloor4
	highTier := EvidenceTier{
		Test:     3.5,
		Receive:  3.5,
		Security: 3.5,
	}
	lvl := highTier.intermediateLevel()
	assert.InDelta(t, EnvelopeTarget*0.9, lvl, 0.01)

	// Direct call to envelopeFloorFor with value < target * 0.8
	floorLow := envelopeFloorFor(2.0, EnvelopeTarget)
	assert.InDelta(t, EnvelopeTarget*0.7, floorLow, 0.01)

	// Direct call to envelopeFloorFor with value >= target * 0.8
	floorHigh := envelopeFloorFor(4.0, EnvelopeTarget)
	assert.InDelta(t, EnvelopeTarget*0.9, floorHigh, 0.01)
}

func TestEnforceOrder_EdgeCases(t *testing.T) {
	enforcer := NewEnforcer(nil)

	// Items with self-comparison, blocked items, and invalid other items
	items := []PlanItem{
		{
			ID:           "ITEM-ACTIVE",
			Status:       StatusActive,
			Priority:     PriorityMedium,
			PriorityTier: TierP2,
		},
		{
			ID:           "ITEM-ACTIVE", // Duplicate ID to hit other.ID == it.ID continue
			Status:       StatusActive,
			Priority:     PriorityMedium,
			PriorityTier: TierP2,
		},
		{
			ID:           "ITEM-BLOCKED",
			Status:       StatusPlanned,
			Priority:     PriorityCritical,
			PriorityTier: TierP0,
			Blocked:      true, // hits other.Blocked continue
		},
		{
			ID:           "ITEM-INVALID",
			Status:       StatusPlanned,
			Priority:     "unknown",
			PriorityTier: "P9", // hits !itemValid(other) continue
		},
	}

	res, err := enforcer.EnforceOrder(items)
	assert.NoError(t, err)
	assert.True(t, res.OK())
}
