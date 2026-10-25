// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package shovelready

import (
	"math"

	"github.com/zqk-os/zqk/pkg/objects"
)

// DefaultScoringWeights provides canonical balance across DoR criteria.
var DefaultScoringWeights = ScoringWeights{
	RequirementWeight: 0.25,
	CriteriaWeight:    0.25,
	EffortWeight:      0.25,
	PersonaWeight:     0.15,
	DependencyWeight:  0.10,
}

// ScoringWeights defines the relative contribution of each readiness dimension.
type ScoringWeights struct {
	RequirementWeight float64
	CriteriaWeight    float64
	EffortWeight      float64
	PersonaWeight     float64
	DependencyWeight  float64
}

// Normalize ensures total weight sums to 1.0 while preserving proportions.
func (w ScoringWeights) Normalize() ScoringWeights {
	sum := w.RequirementWeight + w.CriteriaWeight + w.EffortWeight + w.PersonaWeight + w.DependencyWeight
	if sum <= 0.00001 {
		return DefaultScoringWeights
	}
	return ScoringWeights{
		RequirementWeight: w.RequirementWeight / sum,
		CriteriaWeight:    w.CriteriaWeight / sum,
		EffortWeight:      w.EffortWeight / sum,
		PersonaWeight:     w.PersonaWeight / sum,
		DependencyWeight:  w.DependencyWeight / sum,
	}
}

// ScoreBreakdown details the quantitative readiness calculation for a backlog item.
type ScoreBreakdown struct {
	RequirementScore float64
	CriteriaScore    float64
	EffortScore      float64
	PersonaScore     float64
	DependencyScore  float64
	TotalScore       float64
	PassedThreshold  bool
}

// ReadinessScorer computes standardized readiness scores bounded in [0.0, 1.0].
type ReadinessScorer struct {
	weights   ScoringWeights
	threshold float64
}

// NewReadinessScorer creates a scorer with specified weights and threshold.
func NewReadinessScorer(weights ScoringWeights, threshold float64) *ReadinessScorer {
	if threshold <= 0.0 {
		threshold = 0.80
	} else if threshold > 1.0 {
		threshold = 1.0
	}
	return &ReadinessScorer{
		weights:   weights.Normalize(),
		threshold: threshold,
	}
}

// DefaultReadinessScorer returns a scorer with default balanced weights and 0.80 threshold.
func DefaultReadinessScorer() *ReadinessScorer {
	return NewReadinessScorer(DefaultScoringWeights, 0.80)
}

// Score calculates the readiness score and breakdown for a backlog item map.
func (s *ReadinessScorer) Score(bli map[string]any, isUnblocked bool) ScoreBreakdown {
	if bli == nil {
		return ScoreBreakdown{
			TotalScore:      0.0,
			PassedThreshold: false,
		}
	}

	var bd ScoreBreakdown

	// 1. Requirement or Tech Spec presence
	if hasNonEmptyRefList(bli, objects.FieldKeyRequirementRefs) ||
		hasNonEmptyRefList(bli, objects.FieldKeyTechnicalSpecRefs) {
		bd.RequirementScore = s.weights.RequirementWeight
	}

	// 2. Criteria presence
	if hasNonEmptyRefList(bli, objects.FieldKeyCriteriaRefs) {
		bd.CriteriaScore = s.weights.CriteriaWeight
	}

	// 3. Estimated effort or scope signal
	if hasEstimatedScope(bli) {
		bd.EffortScore = s.weights.EffortWeight
	}

	// 4. Lane or persona assignment
	if hasLaneAssignment(bli) {
		bd.PersonaScore = s.weights.PersonaWeight
	}

	// 5. Dependency unblocked status
	if isUnblocked && !hasUnresolvedBlocker(bli) {
		bd.DependencyScore = s.weights.DependencyWeight
	}

	total := bd.RequirementScore + bd.CriteriaScore + bd.EffortScore + bd.PersonaScore + bd.DependencyScore
	// Clamp strictly between 0.0 and 1.0
	bd.TotalScore = math.Max(0.0, math.Min(1.0, total))
	bd.PassedThreshold = bd.TotalScore >= s.threshold

	return bd
}

// ScoreBatch calculates readiness score breakdowns for a slice of backlog item records.
func (s *ReadinessScorer) ScoreBatch(items []map[string]any, unblockedMap map[string]bool) map[string]ScoreBreakdown {
	results := make(map[string]ScoreBreakdown, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		id, _ := item[objects.FieldKeyID].(string)
		unblocked := true
		if unblockedMap != nil {
			if val, ok := unblockedMap[id]; ok {
				unblocked = val
			}
		}
		results[id] = s.Score(item, unblocked)
	}
	return results
}
