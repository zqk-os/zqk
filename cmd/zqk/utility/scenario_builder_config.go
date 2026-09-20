package utility

import (
	"time"
)

// WithKinds sets the object kinds to generate
func (sb *ScenarioBuilder) WithKinds(kinds ...string) *ScenarioBuilder {
	sb.config.Kinds = kinds
	return sb
}

// WithCount sets the count for a specific kind
func (sb *ScenarioBuilder) WithCount(kind string, count int) *ScenarioBuilder {
	if sb.config.Counts == nil {
		sb.config.Counts = make(map[string]int)
	}
	sb.config.Counts[kind] = count
	return sb
}

// WithDefaultCount sets the default count for all kinds
func (sb *ScenarioBuilder) WithDefaultCount(count int) *ScenarioBuilder {
	sb.config.DefaultCount = count
	return sb
}

// WithDiversity sets the diversity level for a specific kind
func (sb *ScenarioBuilder) WithDiversity(kind string, level int) *ScenarioBuilder {
	if sb.config.Diversity == nil {
		sb.config.Diversity = make(map[string]int)
	}
	if level < 1 {
		level = 1
	}
	if level > 10 {
		level = 10
	}
	sb.config.Diversity[kind] = level
	return sb
}

// WithDefaultDiversity sets the default diversity level
func (sb *ScenarioBuilder) WithDefaultDiversity(level int) *ScenarioBuilder {
	if level < 1 {
		level = 1
	}
	if level > 10 {
		level = 10
	}
	sb.config.DefaultDiversity = level
	return sb
}

// WithLinkProbability sets the probability of creating links between objects
func (sb *ScenarioBuilder) WithLinkProbability(prob float64) *ScenarioBuilder {
	if prob < 0.0 {
		prob = 0.0
	}
	if prob > 1.0 {
		prob = 1.0
	}
	sb.config.LinkProbability = prob
	return sb
}

// WithTimeRange sets the time range for created_at timestamps
func (sb *ScenarioBuilder) WithTimeRange(duration time.Duration) *ScenarioBuilder {
	sb.config.TimeRange = duration
	return sb
}

// WithStartTime sets the start time for temporal distribution
func (sb *ScenarioBuilder) WithStartTime(t time.Time) *ScenarioBuilder {
	sb.config.StartTime = t
	return sb
}
