// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package shovelready

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Standard batch allocation errors.
var (
	ErrNoShovelReadyItems = errors.New("no shovel-ready items meet batch allocation criteria")
	ErrInvalidConstraints = errors.New("invalid batch constraints")
)

// BacklogItemCandidate encapsulates candidate properties for batch allocation.
type BacklogItemCandidate struct {
	ID             string
	RequirementRef string
	WorkstreamRef  string
	PriorityTier   string
	EstimatedHours float64
	ReadinessScore float64
	IsReady        bool
	Raw            map[string]any
}

// ParseEffortHours parses a duration string or numeric estimate into fractional hours.
func ParseEffortHours(effort string) float64 {
	trimmed := strings.ToLower(strings.TrimSpace(effort))
	if trimmed == "" {
		return 2.0 // Default fallback estimate for small items
	}

	if strings.HasSuffix(trimmed, "h") {
		numStr := strings.TrimSpace(strings.TrimSuffix(trimmed, "h"))
		if val, err := strconv.ParseFloat(numStr, 64); err == nil && val > 0 {
			return val
		}
	} else if strings.HasSuffix(trimmed, "d") {
		numStr := strings.TrimSpace(strings.TrimSuffix(trimmed, "d"))
		if val, err := strconv.ParseFloat(numStr, 64); err == nil && val > 0 {
			return val * 8.0 // 8 hours per day
		}
	} else if strings.HasSuffix(trimmed, "m") {
		numStr := strings.TrimSpace(strings.TrimSuffix(trimmed, "m"))
		if val, err := strconv.ParseFloat(numStr, 64); err == nil && val > 0 {
			return val / 60.0
		}
	} else if val, err := strconv.ParseFloat(trimmed, 64); err == nil && val > 0 {
		return val
	}

	// Keyword fallbacks
	switch trimmed {
	case "small", "s":
		return 2.0
	case "medium", "m":
		return 4.0
	case "large", "l":
		return 8.0
	default:
		return 2.0
	}
}

// CandidateFromBLI constructs a BacklogItemCandidate from a raw backlog item map and score.
func CandidateFromBLI(bli map[string]any, score float64, isReady bool) BacklogItemCandidate {
	if bli == nil {
		return BacklogItemCandidate{}
	}

	id, _ := bli[objects.FieldKeyID].(string)
	pTier, _ := bli[objects.FieldKeyPriorityTier].(string)
	if pTier == "" {
		pTier, _ = bli[objects.FieldKeyPriority].(string)
	}

	wsRef, _ := bli[objects.FieldKeyWorkstreamRef].(string)
	if wsRef == "" {
		if list, ok := bli[objects.FieldKeyWorkstreamRefs].([]string); ok && len(list) > 0 {
			wsRef = list[0]
		}
	}

	reqRef := ""
	if list, ok := bli[objects.FieldKeyRequirementRefs].([]string); ok && len(list) > 0 {
		reqRef = list[0]
	}

	effortStr, _ := bli[objects.FieldKeyEstimatedEffort].(string)
	effortHours := ParseEffortHours(effortStr)

	return BacklogItemCandidate{
		ID:             id,
		RequirementRef: reqRef,
		WorkstreamRef:  wsRef,
		PriorityTier:   pTier,
		EstimatedHours: effortHours,
		ReadinessScore: score,
		IsReady:        isReady,
		Raw:            bli,
	}
}

// BatchConstraints sets operational bounds for grouping backlog items into execution cycles.
type BatchConstraints struct {
	MinBatchSize            int
	MaxBatchSize            int
	MaxCapacityHours        float64
	MinReadinessScore       float64
	EnforceSingleWorkstream bool
}

// DefaultBatchConstraints yields canonical 1-cycle bounds (2 to 5 items, up to 16h).
func DefaultBatchConstraints() BatchConstraints {
	return BatchConstraints{
		MinBatchSize:            2,
		MaxBatchSize:            5,
		MaxCapacityHours:        16.0,
		MinReadinessScore:       0.80,
		EnforceSingleWorkstream: true,
	}
}

// Batch represents an atomic execution bundle for a swarm or worker agent.
type Batch struct {
	BatchID       string
	WorkstreamRef string
	Items         []BacklogItemCandidate
	TotalHours    float64
	AverageScore  float64
}

// BatchAllocator groups shovel-ready backlog items into balanced execution batches.
type BatchAllocator struct {
	mu          sync.RWMutex
	constraints BatchConstraints
}

// NewBatchAllocator instantiates a BatchAllocator with given constraints.
func NewBatchAllocator(constraints BatchConstraints) *BatchAllocator {
	return &BatchAllocator{
		constraints: constraints,
	}
}

// Allocate partitions candidates into execution batches and unallocated items.
func (b *BatchAllocator) Allocate(candidates []BacklogItemCandidate) ([]Batch, []BacklogItemCandidate, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.constraints.MaxBatchSize <= 0 {
		return nil, nil, fmt.Errorf("%w: MaxBatchSize must be > 0", ErrInvalidConstraints)
	}

	// Filter ready candidates meeting minimum score
	var eligible []BacklogItemCandidate
	var unallocated []BacklogItemCandidate

	for _, c := range candidates {
		if c.IsReady && c.ReadinessScore >= b.constraints.MinReadinessScore {
			eligible = append(eligible, c)
		} else {
			unallocated = append(unallocated, c)
		}
	}

	if len(eligible) == 0 {
		return nil, unallocated, ErrNoShovelReadyItems
	}

	sortCandidates(eligible)

	if b.constraints.EnforceSingleWorkstream {
		batches, leftover := b.allocateByWorkstream(eligible)
		unallocated = append(unallocated, leftover...)
		return batches, unallocated, nil
	}

	batches, leftover := b.packBatches("global", eligible)
	unallocated = append(unallocated, leftover...)
	return batches, unallocated, nil
}

func (b *BatchAllocator) allocateByWorkstream(candidates []BacklogItemCandidate) ([]Batch, []BacklogItemCandidate) {
	grouped := make(map[string][]BacklogItemCandidate)
	var wsOrder []string

	for _, c := range candidates {
		ws := c.WorkstreamRef
		if ws == "" {
			ws = "default"
		}
		if _, exists := grouped[ws]; !exists {
			wsOrder = append(wsOrder, ws)
		}
		grouped[ws] = append(grouped[ws], c)
	}

	var allBatches []Batch
	var unallocated []BacklogItemCandidate

	batchSeq := 1
	for _, ws := range wsOrder {
		items := grouped[ws]
		batches, leftover := b.packBatches(fmt.Sprintf("%s-batch-%d", ws, batchSeq), items)
		allBatches = append(allBatches, batches...)
		unallocated = append(unallocated, leftover...)
		batchSeq += len(batches)
	}

	return allBatches, unallocated
}

func (b *BatchAllocator) packBatches(batchPrefix string, items []BacklogItemCandidate) ([]Batch, []BacklogItemCandidate) {
	var batches []Batch
	var leftover []BacklogItemCandidate

	currBatch := Batch{
		BatchID:       fmt.Sprintf("%s-%d", batchPrefix, len(batches)+1),
		WorkstreamRef: "",
	}

	for _, item := range items {
		if currBatch.WorkstreamRef == "" && item.WorkstreamRef != "" {
			currBatch.WorkstreamRef = item.WorkstreamRef
		}

		wouldExceedSize := len(currBatch.Items) >= b.constraints.MaxBatchSize
		wouldExceedHours := b.constraints.MaxCapacityHours > 0 &&
			(currBatch.TotalHours+item.EstimatedHours > b.constraints.MaxCapacityHours)

		if (wouldExceedSize || wouldExceedHours) && len(currBatch.Items) >= b.constraints.MinBatchSize {
			currBatch.AverageScore = computeAverageScore(currBatch.Items)
			batches = append(batches, currBatch)
			currBatch = Batch{
				BatchID:       fmt.Sprintf("%s-%d", batchPrefix, len(batches)+1),
				WorkstreamRef: item.WorkstreamRef,
			}
		}

		if len(currBatch.Items) < b.constraints.MaxBatchSize &&
			(b.constraints.MaxCapacityHours <= 0 || currBatch.TotalHours+item.EstimatedHours <= b.constraints.MaxCapacityHours) {
			currBatch.Items = append(currBatch.Items, item)
			currBatch.TotalHours += item.EstimatedHours
		} else {
			leftover = append(leftover, item)
		}
	}

	if len(currBatch.Items) >= b.constraints.MinBatchSize {
		currBatch.AverageScore = computeAverageScore(currBatch.Items)
		batches = append(batches, currBatch)
	} else {
		leftover = append(leftover, currBatch.Items...)
	}

	return batches, leftover
}

// PriorityPlanPacker packs backlog item candidates into a single priority plan cycle.
type PriorityPlanPacker struct {
	mu sync.RWMutex
}

// NewPriorityPlanPacker creates a new PriorityPlanPacker.
func NewPriorityPlanPacker() *PriorityPlanPacker {
	return &PriorityPlanPacker{}
}

// PackPlan selects the optimal candidates for a Priority Plan adhering to constraints.
func (p *PriorityPlanPacker) PackPlan(planID string, workstream string, candidates []BacklogItemCandidate, constraints BatchConstraints) (*Batch, []BacklogItemCandidate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	allocator := NewBatchAllocator(constraints)
	batches, unallocated, err := allocator.Allocate(candidates)
	if err != nil {
		return nil, unallocated, err
	}

	if len(batches) == 0 {
		return nil, unallocated, ErrNoShovelReadyItems
	}

	leadBatch := batches[0]
	leadBatch.BatchID = planID
	if workstream != "" {
		leadBatch.WorkstreamRef = workstream
	}

	// Any subsequent batches fold back into unallocated for upcoming cycles
	for i := 1; i < len(batches); i++ {
		unallocated = append(unallocated, batches[i].Items...)
	}

	return &leadBatch, unallocated, nil
}

func sortCandidates(items []BacklogItemCandidate) {
	sort.SliceStable(items, func(i, j int) bool {
		// Priority Tier: P0 > P1 > P2 > P3
		scoreI := priorityTierScore(items[i].PriorityTier)
		scoreJ := priorityTierScore(items[j].PriorityTier)
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		// Readiness score descending
		if items[i].ReadinessScore != items[j].ReadinessScore {
			return items[i].ReadinessScore > items[j].ReadinessScore
		}
		// Deterministic fallback by ID
		return items[i].ID < items[j].ID
	})
}

func priorityTierScore(tier string) int {
	norm := strings.ToLower(strings.TrimSpace(tier))
	switch {
	case strings.HasPrefix(norm, "p0"):
		return 400
	case strings.HasPrefix(norm, "p1"):
		return 300
	case strings.HasPrefix(norm, "p2"):
		return 200
	case strings.HasPrefix(norm, "p3"):
		return 100
	default:
		return 0
	}
}

func computeAverageScore(items []BacklogItemCandidate) float64 {
	if len(items) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, it := range items {
		sum += it.ReadinessScore
	}
	return sum / float64(len(items))
}
