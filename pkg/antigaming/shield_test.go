package antigaming_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/antigaming"
)

// TestAntiGaming_StaticFloorInvariant verifies baseline shield configuration and empty defaults.
func TestAntiGaming_StaticFloorInvariant(t *testing.T) {
	cfg := antigaming.DefaultShieldConfig()
	assert.Equal(t, 5, cfg.LeafSaturationLimit)
	assert.Equal(t, 1.25, cfg.ExternalFrontierBonus)
	assert.NotEmpty(t, cfg.ExternalFrontierPrefixes)

	// Empty work items yield perfect baseline
	report := antigaming.CalculateAntiGamingShield(nil, cfg)
	require.NotNil(t, report)
	assert.Equal(t, 1.0, report.StrategicCapabilityBreadthIndex)
	assert.Equal(t, 1.0, report.DomainDiversityScore)
	assert.Equal(t, 1.0, report.ExternalFrontierWeight)
	assert.False(t, report.MathematicalSiloDetected)
	assert.False(t, report.DiminishingReturnsApplied)
	assert.Equal(t, "PASS", report.Status)
}

// TestAntiGaming_OperationalDynamicProof verifies breadth scoring, domain diversity, and frontier weighting.
func TestAntiGaming_OperationalDynamicProof(t *testing.T) {
	cfg := antigaming.DefaultShieldConfig()

	// Diverse set of work items touching external frontier packages
	items := []map[string]any{
		{
			"id":            "BLI-1",
			"category":      "network_api",
			"code_location": []string{"pkg/api/client.go"},
		},
		{
			"id":            "BLI-2",
			"category":      "persistence",
			"code_location": []string{"pkg/storage/cas.go"},
		},
		{
			"id":            "BLI-3",
			"category":      "cli_ux",
			"code_location": []string{"pkg/cliapp/runner.go"},
		},
		{
			"id":            "BLI-4",
			"category":      "protocol_adapters",
			"code_location": []string{"pkg/adapters/mcp.go"},
		},
	}

	report := antigaming.CalculateAntiGamingShield(items, cfg)
	require.NotNil(t, report)
	assert.Equal(t, "PASS", report.Status)
	assert.False(t, report.MathematicalSiloDetected)
	assert.False(t, report.DiminishingReturnsApplied)
	assert.GreaterOrEqual(t, report.DomainDiversityScore, 0.8)
	assert.Greater(t, report.ExternalFrontierWeight, 1.0)
	assert.GreaterOrEqual(t, report.StrategicCapabilityBreadthIndex, 0.8)
}

// TestAntiGaming_NegativeAdversarialBoundary ensures agents cannot game convergence via redundant math silos.
func TestAntiGaming_NegativeAdversarialBoundary(t *testing.T) {
	cfg := antigaming.DefaultShieldConfig()
	cfg.LeafSaturationLimit = 5

	// Adversarial Scenario A: Agent attempts to game metrics by generating 20 redundant
	// pure-math items in a single saturated leaf module without touching external frontier.
	var gamedItems []map[string]any
	for i := 1; i <= 20; i++ {
		gamedItems = append(gamedItems, map[string]any{
			"id":            fmt.Sprintf("BLI-DSP-%03d", i),
			"category":      "audio_math_filter",
			"code_location": []string{fmt.Sprintf("pkg/math/dsp/filter_%d.go", i)},
		})
	}

	report := antigaming.CalculateAntiGamingShield(gamedItems, cfg)
	require.NotNil(t, report)

	// Invariant 1: Mathematical Silo MUST be detected because 0% of code touches external frontier
	assert.True(t, report.MathematicalSiloDetected, "expected mathematical silo detection")
	assert.Equal(t, "BLOCKED", report.Status, "expected BLOCKED status for 20-item math silo")

	// Invariant 2: Diminishing returns penalty MUST be applied for 15 excess items beyond saturation limit (5)
	assert.True(t, report.DiminishingReturnsApplied, "expected diminishing returns penalty")
	assert.Contains(t, report.SaturatedDomains, "audio_math_filter")
	assert.Greater(t, report.LeafSaturationPenalty, 0.0)

	// Invariant 3: Strategic capability breadth score MUST be severely depressed despite 20 "completed" items
	assert.Less(t, report.StrategicCapabilityBreadthIndex, 0.3)
	assert.NotEmpty(t, report.Warnings)
}
