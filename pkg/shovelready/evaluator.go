// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package shovelready

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// EvaluatorOption configures custom behavior for ShovelReadyEvaluator.
type EvaluatorOption func(*ShovelReadyEvaluator)

// ShovelReadyEvaluator coordinates backlog item shovel-readiness evaluation.
type ShovelReadyEvaluator struct {
	bypassPrefix     string
	strictEffort     bool
	customValidators []func(map[string]any) []string
}

// WithBypassPrefix overrides the synthetic grooming ID bypass prefix (default "AUTO-BLI-").
func WithBypassPrefix(prefix string) EvaluatorOption {
	return func(e *ShovelReadyEvaluator) {
		e.bypassPrefix = prefix
	}
}

// WithStrictEffort requires explicit estimated_effort without falling back to priority tiers.
func WithStrictEffort(strict bool) EvaluatorOption {
	return func(e *ShovelReadyEvaluator) {
		e.strictEffort = strict
	}
}

// WithCustomValidator adds domain-specific criteria validation checks.
func WithCustomValidator(fn func(map[string]any) []string) EvaluatorOption {
	return func(e *ShovelReadyEvaluator) {
		if fn != nil {
			e.customValidators = append(e.customValidators, fn)
		}
	}
}

// NewEvaluator constructs a new ShovelReadyEvaluator with applied options.
func NewEvaluator(opts ...EvaluatorOption) *ShovelReadyEvaluator {
	e := &ShovelReadyEvaluator{
		bypassPrefix: "AUTO-BLI-",
		strictEffort: false,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Evaluate performs standard shovel-ready evaluation with evaluator-specific options.
func (e *ShovelReadyEvaluator) Evaluate(bli map[string]any) Result {
	if bli == nil {
		return Result{Ready: false, Missing: []string{"object"}}
	}

	id, _ := bli[objects.FieldKeyID].(string)
	if e.bypassPrefix != "" && strings.HasPrefix(strings.TrimSpace(id), e.bypassPrefix) {
		return Result{Ready: true}
	}

	var missing []string
	if !hasNonEmptyRefList(bli, objects.FieldKeyRequirementRefs) &&
		!hasNonEmptyRefList(bli, objects.FieldKeyTechnicalSpecRefs) {
		missing = append(missing, "requirement_refs|technical_spec_refs")
	}
	if !hasNonEmptyRefList(bli, objects.FieldKeyCriteriaRefs) {
		missing = append(missing, objects.FieldKeyCriteriaRefs)
	}

	if e.strictEffort {
		if s, ok := bli[objects.FieldKeyEstimatedEffort].(string); !ok || strings.TrimSpace(s) == "" {
			missing = append(missing, objects.FieldKeyEstimatedEffort)
		}
	} else if !hasEstimatedScope(bli) {
		missing = append(missing, objects.FieldKeyEstimatedEffort+"|scope_signal")
	}

	if !hasLaneAssignment(bli) {
		missing = append(missing, "persona|stakeholders")
	}
	if hasUnresolvedBlocker(bli) {
		missing = append(missing, "unresolved_blocker")
	}

	for _, cv := range e.customValidators {
		if extra := cv(bli); len(extra) > 0 {
			missing = append(missing, extra...)
		}
	}

	return Result{Ready: len(missing) == 0, Missing: missing}
}

// IsReady evaluates whether a single backlog item satisfies shovel-ready requirements.
func (e *ShovelReadyEvaluator) IsReady(bli map[string]any) bool {
	return e.Evaluate(bli).Ready
}

// EvaluateBatch performs shovel-ready evaluation across a slice of backlog item records.
func (e *ShovelReadyEvaluator) EvaluateBatch(items []map[string]any) []Result {
	results := make([]Result, len(items))
	for i, item := range items {
		results[i] = e.Evaluate(item)
	}
	return results
}

// FilterReady partitions input backlog items into those satisfying shovel-readiness.
func (e *ShovelReadyEvaluator) FilterReady(items []map[string]any) []map[string]any {
	var ready []map[string]any
	for _, item := range items {
		if e.IsReady(item) {
			ready = append(ready, item)
		}
	}
	return ready
}
