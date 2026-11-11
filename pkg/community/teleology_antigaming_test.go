package community_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/antigaming"
	"github.com/zqk-os/zqk/pkg/teleology"
)

// TestTeleologyContactInhibition_StaticFloorInvariant satisfies CRIT-1794365724117307000-aae004a6
// Verifying baseline taxonomy contracts, default permissive bypass when unconfigured, and field invariants.
func TestTeleologyContactInhibition_StaticFloorInvariant(t *testing.T) {
	receipt := teleology.EvaluateTeleologicalAlignment(map[string]any{"title": "Core Work Item"}, nil)
	require.NotNil(t, receipt)
	assert.Equal(t, "TELEOLOGICAL_ALIGNED", receipt.Status)
	assert.Equal(t, "MIS-*", receipt.Boundary)

	disabledCfg := &teleology.TaxonomyConfig{Enabled: false}
	receiptDisabled := teleology.EvaluateTeleologicalAlignment(map[string]any{"title": "Core Work Item"}, disabledCfg)
	require.NotNil(t, receiptDisabled)
	assert.Equal(t, "TELEOLOGICAL_ALIGNED", receiptDisabled.Status)

	domains := teleology.ExtractCandidateDomains(map[string]any{
		"category":      "social_syndication",
		"components":    []string{"feed_publisher"},
		"code_location": []string{"pkg/social/publisher.go"},
	})
	assert.Contains(t, domains, "social_syndication")
	assert.Contains(t, domains, "feed_publisher")
	assert.Contains(t, domains, "social")
}

// TestTeleologyContactInhibition_OperationalDynamicProof satisfies CRIT-1794365724117308000-c2ac4138
// Verifying active domain rejection, deterministic receipt format, and semantic drift detection in system alignment.
func TestTeleologyContactInhibition_OperationalDynamicProof(t *testing.T) {
	taxonomy := &teleology.TaxonomyConfig{
		Enabled:           true,
		MissionRef:        "MIS-STARTER-COMMUNITY-001",
		InScope:           []string{"social_syndication", "review_cockpit", "broadcast_mastering"},
		Prohibited:        []string{"analog_instrument_dsp", "synthesizer_oscillators"},
		StrictEnforcement: true,
	}

	// 1. In-Scope Item Passes
	inScopeItem := map[string]any{
		"id":            "BLI-VALID-1",
		"title":         "Deliver OAuth Token Refresh for Social Syndication",
		"category":      "social_syndication",
		"code_location": []string{"pkg/social/oauth.go"},
	}
	passReceipt := teleology.EvaluateTeleologicalAlignment(inScopeItem, taxonomy)
	require.NotNil(t, passReceipt)
	assert.Equal(t, "TELEOLOGICAL_ALIGNED", passReceipt.Status)
	assert.Equal(t, "MIS-STARTER-COMMUNITY-001", passReceipt.Boundary)

	// 2. Prohibited Out-of-Scope Item Fails Closed with Normative Receipt (GH-1071)
	prohibitedItem := map[string]any{
		"id":            "BLI-NEOPLASIA-1",
		"title":         "Synthesizer Oscillator Dual Filter VST",
		"category":      "synthesizer_oscillators",
		"code_location": []string{"pkg/dsp/oscillators.go"},
	}
	failReceipt := teleology.EvaluateTeleologicalAlignment(prohibitedItem, taxonomy)
	require.NotNil(t, failReceipt)
	assert.Equal(t, "TELEOLOGICAL_DRIFT_REFUSED", failReceipt.Status)
	assert.Equal(t, "Domain out of registered taxonomy", failReceipt.Cause)
	assert.Equal(t, "MIS-STARTER-COMMUNITY-001", failReceipt.Boundary)
	assert.Contains(t, failReceipt.ViolatingDomains, "synthesizer_oscillators")

	// 3. System Alignment Semantic Drift Scoring
	workItems := []map[string]any{
		inScopeItem,
		prohibitedItem,
		{
			"id":       "BLI-VALID-2",
			"title":    "Review Cockpit Live Preview Grid",
			"category": "review_cockpit",
		},
	}
	report := teleology.EvaluateWorkItemsTeleologicalDrift(workItems, taxonomy)
	require.NotNil(t, report)
	assert.Equal(t, 3, report.TotalItems)
	assert.Equal(t, 2, report.AlignedCount)
	assert.Equal(t, 1, report.DriftCount)
	assert.Equal(t, 33.33, report.DriftPercentage)
	assert.Equal(t, 66.67, report.TeleologicalScore)
	assert.Equal(t, "TELEOLOGICAL_DRIFT_DETECTED", report.Status)
	assert.Len(t, report.DriftReceipts, 1)
	assert.Equal(t, "BLI-NEOPLASIA-1", report.DriftReceipts[0]["id"])
}

// TestTeleologyContactInhibition_NegativeAdversarialBoundary satisfies CRIT-1794365724117309000-dce42ef1
// Verifying fail-closed contact inhibition against adversarial domain masking and evasive goal attachments.
func TestTeleologyContactInhibition_NegativeAdversarialBoundary(t *testing.T) {
	taxonomy := &teleology.TaxonomyConfig{
		Enabled:           true,
		MissionRef:        "MIS-ASMM-001",
		InScope:           []string{"social_syndication", "review_cockpit"},
		Prohibited:        []string{"analog_instrument_dsp", "synthesizer_oscillators"},
		StrictEnforcement: true,
	}

	// Attack Vector 1: Valid GoalRefs camouflage with prohibited code location
	evasiveGoalAttached := map[string]any{
		"id":            "BLI-ATTACK-1",
		"title":         "Legitimate Marketing Goal Connector",
		"goal_refs":     []string{"GOAL-ROOT-001"},
		"code_location": []string{"pkg/analog_instrument_dsp/distortion.go"},
	}
	receipt1 := teleology.EvaluateTeleologicalAlignment(evasiveGoalAttached, taxonomy)
	assert.Equal(t, "TELEOLOGICAL_DRIFT_REFUSED", receipt1.Status)
	assert.Equal(t, "Domain out of registered taxonomy", receipt1.Cause)

	// Attack Vector 2: Trojan Horse multi-component injection
	trojanItem := map[string]any{
		"id":         "BLI-ATTACK-2",
		"title":      "Review Cockpit Audio Plugin",
		"category":   "review_cockpit",
		"components": []string{"review_cockpit", "synthesizer_oscillators"},
	}
	receipt2 := teleology.EvaluateTeleologicalAlignment(trojanItem, taxonomy)
	assert.Equal(t, "TELEOLOGICAL_DRIFT_REFUSED", receipt2.Status)

	// Attack Vector 3: Punctuation and normalization evasions
	normalizedItem := map[string]any{
		"id":       "BLI-ATTACK-3",
		"title":    "Analog-Instrument-DSP Distortion Simulator",
		"category": "analog-instrument-dsp",
	}
	receipt3 := teleology.EvaluateTeleologicalAlignment(normalizedItem, taxonomy)
	assert.Equal(t, "TELEOLOGICAL_DRIFT_REFUSED", receipt3.Status)
}

// TestAntiGamingMetricShield_StaticFloorInvariant satisfies CRIT-1794365733135949000-6e324906
// Verifying anti-gaming configuration defaults and baseline convergence outputs.
func TestAntiGamingMetricShield_StaticFloorInvariant(t *testing.T) {
	cfg := antigaming.DefaultShieldConfig()
	assert.Equal(t, 5, cfg.LeafSaturationLimit)
	assert.Equal(t, 1.25, cfg.ExternalFrontierBonus)
	assert.NotEmpty(t, cfg.ExternalFrontierPrefixes)

	report := antigaming.CalculateAntiGamingShield(nil, cfg)
	require.NotNil(t, report)
	assert.Equal(t, 1.0, report.StrategicCapabilityBreadthIndex)
	assert.Equal(t, 1.0, report.DomainDiversityScore)
	assert.Equal(t, 1.0, report.ExternalFrontierWeight)
	assert.False(t, report.MathematicalSiloDetected)
	assert.False(t, report.DiminishingReturnsApplied)
	assert.Equal(t, "PASS", report.Status)
}

// TestAntiGamingMetricShield_OperationalDynamicProof satisfies CRIT-1794365733135950000-0c18b1e3
// Verifying capability breadth weighting, domain diversity scoring, and external frontier bonuses.
func TestAntiGamingMetricShield_OperationalDynamicProof(t *testing.T) {
	cfg := antigaming.DefaultShieldConfig()

	diverseFrontierItems := []map[string]any{
		{
			"id":            "BLI-FRONTIER-1",
			"category":      "network_api",
			"code_location": []string{"pkg/api/client.go"},
		},
		{
			"id":            "BLI-FRONTIER-2",
			"category":      "persistence",
			"code_location": []string{"pkg/storage/cas.go"},
		},
		{
			"id":            "BLI-FRONTIER-3",
			"category":      "cli_ux",
			"code_location": []string{"pkg/cliapp/runner.go"},
		},
		{
			"id":            "BLI-FRONTIER-4",
			"category":      "protocol_adapters",
			"code_location": []string{"pkg/adapters/mcp.go"},
		},
	}

	report := antigaming.CalculateAntiGamingShield(diverseFrontierItems, cfg)
	require.NotNil(t, report)
	assert.Equal(t, "PASS", report.Status)
	assert.False(t, report.MathematicalSiloDetected)
	assert.False(t, report.DiminishingReturnsApplied)
	assert.GreaterOrEqual(t, report.DomainDiversityScore, 0.8)
	assert.Greater(t, report.ExternalFrontierWeight, 1.0)
	assert.GreaterOrEqual(t, report.StrategicCapabilityBreadthIndex, 0.8)
}

// TestAntiGamingMetricShield_NegativeAdversarialBoundary satisfies CRIT-1794365733135951000-84958ea7
// Verifying detection and failure of redundant algorithmic silos and saturated leaf packages.
func TestAntiGamingMetricShield_NegativeAdversarialBoundary(t *testing.T) {
	cfg := antigaming.DefaultShieldConfig()
	cfg.LeafSaturationLimit = 5

	// 20 redundant pure-math items in a single leaf module attempting to inflate metrics
	var gamedItems []map[string]any
	for i := 1; i <= 20; i++ {
		gamedItems = append(gamedItems, map[string]any{
			"id":            fmt.Sprintf("BLI-SILO-%03d", i),
			"category":      "audio_math_filter",
			"code_location": []string{fmt.Sprintf("pkg/math/dsp/filter_%d.go", i)},
		})
	}

	report := antigaming.CalculateAntiGamingShield(gamedItems, cfg)
	require.NotNil(t, report)

	// Invariant 1: Mathematical Silo Flagged (0% external frontier)
	assert.True(t, report.MathematicalSiloDetected, "must detect pure mathematical silo")
	assert.Equal(t, "BLOCKED", report.Status, "must report BLOCKED for multi-item math silo")

	// Invariant 2: Diminishing returns penalty enforced
	assert.True(t, report.DiminishingReturnsApplied, "must apply diminishing returns on leaf saturation")
	assert.Contains(t, report.SaturatedDomains, "audio_math_filter")
	assert.Greater(t, report.LeafSaturationPenalty, 0.0)

	// Invariant 3: Score severely penalized
	assert.Less(t, report.StrategicCapabilityBreadthIndex, 0.3)
	assert.NotEmpty(t, report.Warnings)
}
