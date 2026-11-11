package teleology_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/teleology"
)

// TestTeleology_StaticFloorInvariant verifies baseline taxonomy contracts and default non-interfering invariants.
func TestTeleology_StaticFloorInvariant(t *testing.T) {
	// When disabled or nil, alignment always succeeds without interference
	receiptNil := teleology.EvaluateTeleologicalAlignment(map[string]any{"title": "Arbitrary Work"}, nil)
	assert.Equal(t, "TELEOLOGICAL_ALIGNED", receiptNil.Status)
	assert.Equal(t, "MIS-*", receiptNil.Boundary)

	cfgDisabled := &teleology.TaxonomyConfig{
		Enabled: false,
	}
	receiptDisabled := teleology.EvaluateTeleologicalAlignment(map[string]any{"title": "Arbitrary Work"}, cfgDisabled)
	assert.Equal(t, "TELEOLOGICAL_ALIGNED", receiptDisabled.Status)

	// Candidate domain extraction baseline
	domains := teleology.ExtractCandidateDomains(map[string]any{
		"category":      "social_syndication",
		"components":    []string{"feed_publisher", "oauth_connector"},
		"code_location": []string{"pkg/social/publisher.go"},
		"title":         "[marketing] Campaign Scheduler",
	})
	assert.Contains(t, domains, "social_syndication")
	assert.Contains(t, domains, "feed_publisher")
	assert.Contains(t, domains, "oauth_connector")
	assert.Contains(t, domains, "social")
	assert.Contains(t, domains, "marketing")
}

// TestTeleology_OperationalDynamicProof verifies active taxonomic filtering, rejection receipts, and scoring.
func TestTeleology_OperationalDynamicProof(t *testing.T) {
	taxonomy := &teleology.TaxonomyConfig{
		Enabled:           true,
		MissionRef:        "MIS-STARTER-COMMUNITY-001",
		InScope:           []string{"social_syndication", "review_cockpit", "broadcast_mastering"},
		Prohibited:        []string{"analog_instrument_dsp", "synthesizer_oscillators"},
		StrictEnforcement: true,
	}

	// 1. In-Scope Item Passes
	inScopeItem := map[string]any{
		"id":            "BLI-001",
		"title":         "Deliver OAuth Token Refresh for Social Syndication",
		"category":      "social_syndication",
		"code_location": []string{"pkg/social/oauth.go"},
	}
	receiptPass := teleology.EvaluateTeleologicalAlignment(inScopeItem, taxonomy)
	require.NotNil(t, receiptPass)
	assert.Equal(t, "TELEOLOGICAL_ALIGNED", receiptPass.Status)
	assert.Equal(t, "MIS-STARTER-COMMUNITY-001", receiptPass.Boundary)

	// 2. Out-of-Scope / Prohibited Item Fails Closed
	prohibitedItem := map[string]any{
		"id":            "BLI-666",
		"title":         "Synthesizer Oscillator Dual Filter VST",
		"category":      "synthesizer_oscillators",
		"code_location": []string{"pkg/dsp/oscillators.go"},
	}
	receiptFail := teleology.EvaluateTeleologicalAlignment(prohibitedItem, taxonomy)
	require.NotNil(t, receiptFail)
	assert.Equal(t, "TELEOLOGICAL_DRIFT_REFUSED", receiptFail.Status)
	assert.Equal(t, "Domain out of registered taxonomy", receiptFail.Cause)
	assert.Equal(t, "MIS-STARTER-COMMUNITY-001", receiptFail.Boundary)
	assert.Contains(t, receiptFail.ViolatingDomains, "synthesizer_oscillators")

	// 3. Batch Drift Evaluation & Alignment Scoring
	items := []map[string]any{
		inScopeItem,
		prohibitedItem,
		{
			"id":       "BLI-002",
			"title":    "Review Cockpit Live Preview Grid",
			"category": "review_cockpit",
		},
	}
	report := teleology.EvaluateWorkItemsTeleologicalDrift(items, taxonomy)
	require.NotNil(t, report)
	assert.Equal(t, 3, report.TotalItems)
	assert.Equal(t, 2, report.AlignedCount)
	assert.Equal(t, 1, report.DriftCount)
	assert.Equal(t, 33.33, report.DriftPercentage)
	assert.Equal(t, 66.67, report.TeleologicalScore)
	assert.Equal(t, "TELEOLOGICAL_DRIFT_DETECTED", report.Status)
	assert.Len(t, report.DriftReceipts, 1)
	assert.Equal(t, "BLI-666", report.DriftReceipts[0]["id"])
}

// TestTeleology_NegativeAdversarialBoundary ensures evasive objects cannot bypass contact inhibition.
func TestTeleology_NegativeAdversarialBoundary(t *testing.T) {
	taxonomy := &teleology.TaxonomyConfig{
		Enabled:           true,
		MissionRef:        "MIS-ASMM-001",
		InScope:           []string{"social_syndication", "review_cockpit"},
		Prohibited:        []string{"analog_instrument_dsp", "synthesizer_oscillators"},
		StrictEnforcement: true,
	}

	// Adversarial Scenario A: Disguised Prohibited Domain in Code Location
	// Object attempts to attach goal_refs and legitimate title, but points code to prohibited DSP
	evasiveItemA := map[string]any{
		"id":            "BLI-EVASIVE-1",
		"title":         "Social Media Feed Transformer",
		"goal_refs":     []string{"GOAL-ROOT-001"},
		"code_location": []string{"pkg/analog_instrument_dsp/distortion.go"},
	}
	receiptA := teleology.EvaluateTeleologicalAlignment(evasiveItemA, taxonomy)
	assert.Equal(t, "TELEOLOGICAL_DRIFT_REFUSED", receiptA.Status)
	assert.Equal(t, "Domain out of registered taxonomy", receiptA.Cause)

	// Adversarial Scenario B: Normalization & Hyphenation Variations
	// Object uses hyphenated 'analog-instrument-dsp' or uppercase to bypass naive string equality
	evasiveItemB := map[string]any{
		"id":         "BLI-EVASIVE-2",
		"title":      "ANALOG-INSTRUMENT-DSP Module",
		"components": []string{"analog-instrument-dsp"},
	}
	receiptB := teleology.EvaluateTeleologicalAlignment(evasiveItemB, taxonomy)
	assert.Equal(t, "TELEOLOGICAL_DRIFT_REFUSED", receiptB.Status)

	// Adversarial Scenario C: Trojan Horse (Mixed in-scope domain + prohibited domain)
	// Declares legitimate category but smuggles prohibited component
	trojanItem := map[string]any{
		"id":         "BLI-TROJAN",
		"title":      "Review Cockpit Waveform Visualizer",
		"category":   "review_cockpit",
		"components": []string{"review_cockpit", "synthesizer_oscillators"},
	}
	receiptC := teleology.EvaluateTeleologicalAlignment(trojanItem, taxonomy)
	// Fail-closed invariant: prohibited domain takes absolute precedence
	assert.Equal(t, "TELEOLOGICAL_DRIFT_REFUSED", receiptC.Status)
}
