package cef

import (
	"fmt"
	"sort"
)

// Priority Order Enforcement Requirement — CEF architecture follow-on part 2.
//
// A priority plan must not execute work out of order: an item that is
// execution-facing (active / in_progress) may not outrank a still-open
// (planned) item with a strictly higher priority that is unblocked. This
// module gives callers a pure, storage-free decision core so CLI, MCP tooling,
// and lifecycle hooks can all enforce the same ordering semantics.

// Priorities, tiers, and statuses are kept as plain-string constants (FieldKey
// / env literals style) so the enforcement core never imports the object
// graph and stays testable in isolation.
const (
	PriorityCritical = "critical"
	PriorityHigh     = "high"
	PriorityMedium   = "medium"
	PriorityLow      = "low"

	TierP0 = "P0"
	TierP1 = "P1"
	TierP2 = "P2"
	TierP3 = "P3"

	StatusPlanned    = "planned"
	StatusActive     = "active"
	StatusInProgress = "in_progress"
	StatusComplete   = "complete"
	StatusArchived   = "archived"
)

// Violation codes emitted by the enforcer.
const (
	CodePriorityOrderInverted = "priority_order_inverted"
	CodePriorityPairing       = "priority_pairing"
	CodeMissingPriority       = "missing_priority"
)

// Execution-facing statuses: the item is actually consuming delivery capacity.
var executionFacing = map[string]bool{
	StatusActive:     true,
	StatusInProgress: true,
}

// rankOf maps a priority band to its enforcement rank (0 = highest).
var priorityRank = map[string]int{
	PriorityCritical: 0,
	PriorityHigh:     1,
	PriorityMedium:   2,
	PriorityLow:      3,
}

// tierRank maps a tier to its rank (0 = highest), keeping tiers comparable.
var tierRank = map[string]int{
	TierP0: 0,
	TierP1: 1,
	TierP2: 2,
	TierP3: 3,
}

// PlanItem is the minimal projection of a priority-plan membership row that
// the order enforcer needs. Callers extract it from stored objects so the
// enforcer stays storage-free.
type PlanItem struct {
	ID           string
	Status       string
	Priority     string
	PriorityTier string
	Blocked      bool
}

// Violation is a single ordering-semantics breach detected on one item.
type Violation struct {
	Code   string
	ItemID string
	Detail string
}

// Result is the outcome of an EnforceOrder pass over a plan's items.
type Result struct {
	Violations []Violation
}

// OK reports whether no violations were found.
func (r *Result) OK() bool {
	return len(r.Violations) == 0
}

// Enforcer validates priority ordering. The optional dependency slot is kept
// for future storage-backed lookups (blocked-graph proof) and is unused by
// the pure core.
type Enforcer struct {
	// reserved for storage-backed dependency proof; nil today.
	_ interface{}
}

// NewEnforcer returns a fresh Enforcer.
func NewEnforcer(_ interface{}) *Enforcer { return &Enforcer{} }

// EnforceOrder validates per-item priority hygiene and head-of-line ordering.
func (e *Enforcer) EnforceOrder(items []PlanItem) (Result, error) {
	res := Result{}

	for _, it := range items {
		if executionFacing[it.Status] {
			// 1) Execution-facing items must carry a complete, legitimate pair.
			if it.Priority == "" || it.PriorityTier == "" {
				res.Violations = append(res.Violations, Violation{
					Code:   CodeMissingPriority,
					ItemID: it.ID,
					Detail: fmt.Sprintf("execution-facing item must carry both priority and priority_tier (status %q)", it.Status),
				})
			} else if !Paired(it.Priority, it.PriorityTier) {
				res.Violations = append(res.Violations, Violation{
					Code:   CodePriorityPairing,
					ItemID: it.ID,
					Detail: fmt.Sprintf("priority %q does not pair with tier %q", it.Priority, it.PriorityTier),
				})
			}
		}

		// 2) Head-of-line: an execution-facing item that outranks (lower rank =
		// better) an open, unblocked, planned item of strictly higher priority
		// inverts the required delivery order.
		if executionFacing[it.Status] && itemValid(it) {
			myRank := PriorityRank(it.Priority)
			for _, other := range items {
				if other.ID == it.ID {
					continue
				}
				if other.Status != StatusPlanned || other.Blocked || !itemValid(other) {
					continue
				}
				if PriorityRank(other.Priority) < myRank {
					res.Violations = append(res.Violations, Violation{
						Code:   CodePriorityOrderInverted,
						ItemID: it.ID,
						Detail: fmt.Sprintf("item outranks open higher-priority planned item %s (%s/%s)", other.ID, other.Priority, other.PriorityTier),
					})
					break // one violation per in-order offender is enough signal
				}
			}
		}
	}

	sort.Slice(res.Violations, func(i, j int) bool { return res.Violations[i].ItemID < res.Violations[j].ItemID })
	return res, nil
}

// itemValid reports whether an item carries a complete, pair-consistent
// priority — only comparable items participate in ordering math.
func itemValid(it PlanItem) bool {
	return PriorityRank(it.Priority) >= 0 && TierRank(it.PriorityTier) >= 0 && Paired(it.Priority, it.PriorityTier)
}

// PriorityRank returns the enforcement rank of a priority band (0 = highest).
// An unknown band ranks -1 (invalid).
func PriorityRank(priority string) int {
	r, ok := priorityRank[priority]
	if !ok {
		return -1
	}
	return r
}

// TierRank returns the enforcement rank of a tier (0 = highest).
// An unknown tier ranks -1 (invalid).
func TierRank(tier string) int {
	r, ok := tierRank[tier]
	if !ok {
		return -1
	}
	return r
}

// Paired reports whether a priority band and tier form a legitimate pair
// (P0↔critical, P1↔high, P2↔medium, P3↔low).
func Paired(priority, tier string) bool {
	pr, tr := PriorityRank(priority), TierRank(tier)
	return pr >= 0 && pr == tr
}
