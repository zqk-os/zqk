package steer

import (
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/vector"
)

// Intensity keywords
var (
	urgentKeywords = []string{
		"urgently", "strictly", "critical", "critically", "immediately",
		"heavily", "maximum", "asap", "vital", "extremely", "drastically",
	}
	slightKeywords = []string{
		"slightly", "gently", "minor", "somewhat", "minimally", "lightly", "bit",
	}
	moderateKeywords = []string{
		"moderately", "balanced", "regular", "normally", "standard",
	}

	reduceKeywords = []string{
		"decrease", "reduce", "relax", "minimize", "lower", "less",
		"down", "drop", "suppress", "loosen", "ease", "deprioritize",
	}
	boostKeywords = []string{
		"boost", "increase", "prioritize", "focus on", "focus", "enhance",
		"maximize", "more", "elevate", "up", "speed up", "harden",
		"strengthen", "raise", "emphasize", "advance",
	}
)

// Dimension keyword mapping
var dimensionKeywordMap = map[vector.Dimension][]string{
	vector.DimensionSecurity: {
		"security", "vulnerability", "vuln", "auth", "secret", "permissions",
		"leak", "cve", "exploit", "sanitize", "firewall", "harden", "secure",
	},
	vector.DimensionReliability: {
		"reliability", "stability", "stable", "flake", "retry", "resilient",
		"robust", "fault", "failover", "recovery", "error", "test", "tests",
		"coverage", "unit-test", "verify", "verification", "assert", "qa", "tdd",
	},
	vector.DimensionEfficiency: {
		"speed", "latency", "performance", "fast", "throughput", "memory",
		"cpu", "tokens", "token", "budget", "cost", "quota", "efficiency",
		"fast-path", "conserve",
	},
	vector.DimensionReadability: {
		"docstring", "docstrings", "docs", "documentation", "comments",
		"readability", "clean-code", "clarity", "style",
	},
	vector.DimensionGovernance: {
		"governance", "compliance", "policy", "license", "audit",
		"gate", "rules", "invariant", "check-valves",
	},
	vector.DimensionCapability: {
		"capability", "feature", "functional", "expand", "expand-scope",
		"breadth", "features", "capabilities",
	},
	vector.DimensionPortability: {
		"portability", "cross-platform", "os-agnostic", "neutrality",
	},
}

var wordSplitRegex = regexp.MustCompile(`[^\w\-]+`)

// ParseIntent parses a natural language intent string into a SteeringIntent.
func ParseIntent(raw string) SteeringIntent {
	text := strings.ToLower(strings.TrimSpace(raw))
	tokens := wordSplitRegex.Split(text, -1)

	// 1. Detect intensity
	intensity := 1.0 // default
	for _, token := range tokens {
		if containsString(urgentKeywords, token) {
			intensity = 1.5
			break
		}
		if containsString(slightKeywords, token) {
			intensity = 0.5
			break
		}
		if containsString(moderateKeywords, token) {
			intensity = 1.0
			break
		}
	}

	// 2. Detect direction (boost vs reduce)
	direction := "boost"
	for _, token := range tokens {
		if containsString(reduceKeywords, token) {
			direction = "reduce"
			break
		}
	}

	// 3. Detect target dimensions and keywords
	matchedDimensions := make(map[vector.Dimension]bool)
	var matchedKeywords []string

	for dim, kwList := range dimensionKeywordMap {
		for _, kw := range kwList {
			if strings.Contains(text, kw) {
				matchedDimensions[dim] = true
				matchedKeywords = append(matchedKeywords, kw)
			}
		}
	}

	// If no dimensions were explicitly recognized, default to reliability & security if words are ambiguous
	if len(matchedDimensions) == 0 {
		matchedDimensions[vector.DimensionReliability] = true
	}

	dimensions := make([]vector.Dimension, 0, len(matchedDimensions))
	targetVectorIDs := make([]string, 0, len(matchedDimensions))
	for dim := range matchedDimensions {
		dimensions = append(dimensions, dim)
		targetVectorIDs = append(targetVectorIDs, "VEC-"+strings.ToUpper(string(dim)))
	}

	return SteeringIntent{
		RawText:           raw,
		Dimensions:        dimensions,
		TargetVectorIDs:   targetVectorIDs,
		Intensity:         intensity,
		Direction:         direction,
		ExtractedKeywords: matchedKeywords,
	}
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
