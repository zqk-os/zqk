package antigaming

import (
	"math"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ShieldConfig tunes the Anti-Gaming Metric Shield thresholds.
type ShieldConfig struct {
	LeafSaturationLimit      int      `yaml:"leaf_saturation_limit" json:"leaf_saturation_limit"`
	ExternalFrontierBonus    float64  `yaml:"external_frontier_bonus" json:"external_frontier_bonus"`
	ExternalFrontierPrefixes []string `yaml:"external_frontier_prefixes" json:"external_frontier_prefixes"`
}

// DefaultShieldConfig returns standard baseline parameters.
func DefaultShieldConfig() ShieldConfig {
	return ShieldConfig{
		LeafSaturationLimit:   5,
		ExternalFrontierBonus: 1.25,
		ExternalFrontierPrefixes: []string{
			"pkg/adapters",
			"pkg/mcp",
			"pkg/api",
			"pkg/storage",
			"pkg/cliapp",
			"pkg/community",
			"pkg/daemon",
			"cmd/",
		},
	}
}

// AntiGamingReport represents the evaluation output of the convergence anti-gaming shield.
type AntiGamingReport struct {
	StrategicCapabilityBreadthIndex float64        `json:"strategic_capability_breadth_index"`
	DomainDiversityScore            float64        `json:"domain_diversity_score"`
	LeafSaturationPenalty           float64        `json:"leaf_saturation_penalty"`
	ExternalFrontierWeight          float64        `json:"external_frontier_weight"`
	MathematicalSiloDetected        bool           `json:"mathematical_silo_detected"`
	DiminishingReturnsApplied       bool           `json:"diminishing_returns_applied"`
	DomainDistribution              map[string]int `json:"domain_distribution"`
	SaturatedDomains                []string       `json:"saturated_domains,omitempty"`
	Status                          string         `json:"status"` // "PASS", "WARNING", "BLOCKED"
	Warnings                        []string       `json:"warnings,omitempty"`
}

// CalculateAntiGamingShield analyzes work items for strategic breadth and anti-gaming compliance.
func CalculateAntiGamingShield(items []map[string]any, cfg ShieldConfig) *AntiGamingReport {
	if cfg.LeafSaturationLimit <= 0 {
		cfg.LeafSaturationLimit = 5
	}
	if cfg.ExternalFrontierBonus <= 0 {
		cfg.ExternalFrontierBonus = 1.25
	}
	if len(cfg.ExternalFrontierPrefixes) == 0 {
		cfg = DefaultShieldConfig()
	}

	report := &AntiGamingReport{
		DomainDistribution:     make(map[string]int),
		ExternalFrontierWeight: 1.0,
		Status:                 "PASS",
	}

	if len(items) == 0 {
		report.StrategicCapabilityBreadthIndex = 1.0
		report.DomainDiversityScore = 1.0
		return report
	}

	externalFrontierCount := 0
	totalCodeLocations := 0

	for _, item := range items {
		locs := extractLocations(item)
		totalCodeLocations += len(locs)

		domains := extractItemDomains(item, locs)
		for _, d := range domains {
			report.DomainDistribution[d]++
		}

		for _, l := range locs {
			if isExternalFrontier(l, cfg.ExternalFrontierPrefixes) {
				externalFrontierCount++
				break
			}
		}
	}

	// 1. Calculate Domain Diversity Score
	uniqueDomains := len(report.DomainDistribution)
	if uniqueDomains == 0 {
		uniqueDomains = 1
	}
	// Scale domain diversity: target at least 2 distinct domains for healthy breadth in larger sets
	diversityRatio := float64(uniqueDomains) / math.Max(1.0, math.Min(5.0, float64(len(items))))
	if diversityRatio > 1.0 {
		diversityRatio = 1.0
	}
	report.DomainDiversityScore = math.Round(diversityRatio*100) / 100

	// 2. Leaf Saturation & Diminishing Marginal Returns Calculation
	totalSaturationPenalty := 0.0
	for domain, count := range report.DomainDistribution {
		if count > cfg.LeafSaturationLimit {
			report.SaturatedDomains = append(report.SaturatedDomains, domain)
			excess := float64(count - cfg.LeafSaturationLimit)
			// Logarithmic diminishing returns penalty:
			// higher saturation leads to proportionally higher penalty on marginal yield
			penalty := math.Log2(1.0+excess) / 10.0
			totalSaturationPenalty += penalty
		}
	}

	if totalSaturationPenalty > 0 {
		report.DiminishingReturnsApplied = true
		if totalSaturationPenalty > 0.5 {
			totalSaturationPenalty = 0.5
		}
		report.LeafSaturationPenalty = math.Round(totalSaturationPenalty*100) / 100
		report.Warnings = append(report.Warnings, "Diminishing returns applied due to domain saturation: "+strings.Join(report.SaturatedDomains, ", "))
	}

	// 3. External Integration Frontier Weighting & Mathematical Silo Detection
	if totalCodeLocations > 0 && externalFrontierCount == 0 {
		// 100% of code locations touch internal leaf/pure-math modules with zero external frontier call paths
		report.MathematicalSiloDetected = true
		report.Status = "WARNING"
		report.Warnings = append(report.Warnings, "Mathematical silo detected: zero call paths to external integration frontier (network, storage, CLI, or adapters)")
	} else if externalFrontierCount > 0 {
		frontierRatio := float64(externalFrontierCount) / float64(len(items))
		report.ExternalFrontierWeight = math.Round((1.0+(frontierRatio*(cfg.ExternalFrontierBonus-1.0)))*100) / 100
	}

	// 4. Strategic Capability Breadth Index computation
	// Breadth = (Diversity * FrontierWeight) - SaturationPenalty
	breadth := (report.DomainDiversityScore * report.ExternalFrontierWeight) - report.LeafSaturationPenalty
	if report.MathematicalSiloDetected {
		breadth *= 0.5
	}
	if breadth < 0 {
		breadth = 0
	}
	if breadth > 1.0 {
		breadth = 1.0
	}
	report.StrategicCapabilityBreadthIndex = math.Round(breadth*100) / 100

	if report.MathematicalSiloDetected && len(items) >= 3 {
		report.Status = "BLOCKED"
	}

	return report
}

func extractLocations(item map[string]any) []string {
	var out []string
	if locs, ok := item[objects.FieldKeyCodeLocation].([]any); ok {
		for _, l := range locs {
			out = append(out, strings.TrimSpace(stringVal(l)))
		}
	} else if locs, ok := item[objects.FieldKeyCodeLocation].([]string); ok {
		for _, l := range locs {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func extractItemDomains(item map[string]any, locs []string) []string {
	domains := make(map[string]bool)

	if cat, ok := item[objects.FieldKeyCategory].(string); ok && strings.TrimSpace(cat) != "" {
		domains[strings.TrimSpace(cat)] = true
	}

	if comps, ok := item[objects.FieldKeyComponents].([]any); ok {
		for _, c := range comps {
			if s := strings.TrimSpace(stringVal(c)); s != "" {
				domains[s] = true
			}
		}
	} else if comps, ok := item[objects.FieldKeyComponents].([]string); ok {
		for _, c := range comps {
			if s := strings.TrimSpace(c); s != "" {
				domains[s] = true
			}
		}
	}

	for _, l := range locs {
		clean := strings.TrimPrefix(l, "./")
		clean = strings.TrimPrefix(clean, "pkg/")
		parts := strings.Split(clean, "/")
		if len(parts) > 0 && parts[0] != "" {
			domains[parts[0]] = true
		}
	}

	if len(domains) == 0 {
		domains["default"] = true
	}

	res := make([]string, 0, len(domains))
	for d := range domains {
		res = append(res, d)
	}
	return res
}

func isExternalFrontier(loc string, frontierPrefixes []string) bool {
	loc = strings.TrimPrefix(loc, "./")
	for _, p := range frontierPrefixes {
		p = strings.TrimPrefix(p, "./")
		if strings.HasPrefix(loc, p) {
			return true
		}
	}
	return false
}

func stringVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
