package cef

import (
	"testing"
)

// CEF architecture follow-on part 2 — Priority Order Enforcement Requirement.
//
// Invariants under test:
//  1. Every execution-facing item (active / in_progress) carries a legitimate
//     priority band and tier, and the band/tier pairing is consistent
//     (P0↔critical, P1↔high, P2↔medium, P3↔low).
//  2. Priority ordering within a plan: a lower-priority (rank > 0) item must
//     not be execution-facing while a higher-priority item in the SAME plan
//     remains open (planned) and unblocked.
//  3. Complete/archived items never participate in the ordering comparison.
func TestEnforcePriorityOrder_HappyPathOrdered(t *testing.T) {
	items := []PlanItem{
		{ID: "bli-p0", Status: StatusInProgress, Priority: PriorityCritical, PriorityTier: TierP0},
		{ID: "bli-p1", Status: StatusPlanned, Priority: PriorityHigh, PriorityTier: TierP1},
		{ID: "bli-done", Status: StatusComplete, Priority: PriorityHigh, PriorityTier: TierP1},
	}
	res, err := NewEnforcer(nil).EnforceOrder(items)
	if err != nil {
		t.Fatalf("EnforceOrder returned error: %v", err)
	}
	if !res.OK() {
		t.Fatalf("expected no violations, got: %v", res.Violations)
	}
}

func TestEnforcePriorityOrder_LowBeforeHighInProgress(t *testing.T) {
	items := []PlanItem{
		{ID: "bli-low", Status: StatusInProgress, Priority: PriorityLow, PriorityTier: TierP3},
		{ID: "bli-high", Status: StatusPlanned, Priority: PriorityHigh, PriorityTier: TierP1, Blocked: false},
	}
	res, err := NewEnforcer(nil).EnforceOrder(items)
	if err != nil {
		t.Fatalf("EnforceOrder returned error: %v", err)
	}
	if res.OK() {
		t.Fatal("expected a priority-order violation, got none")
	}
	if res.Violations[0].Code != CodePriorityOrderInverted {
		t.Fatalf("wrong violation code %q, want %q", res.Violations[0].Code, CodePriorityOrderInverted)
	}
	if res.Violations[0].ItemID != "bli-low" {
		t.Fatalf("violation must be recorded on the in-flower item bli-low, got %q", res.Violations[0].ItemID)
	}
}

func TestEnforcePriorityOrder_BlockedHighExempt(t *testing.T) {
	items := []PlanItem{
		{ID: "bli-medium", Status: StatusInProgress, Priority: PriorityMedium, PriorityTier: TierP2},
		{ID: "bli-high-blocked", Status: StatusPlanned, Priority: PriorityHigh, PriorityTier: TierP1, Blocked: true},
	}
	res, err := NewEnforcer(nil).EnforceOrder(items)
	if err != nil {
		t.Fatalf("EnforceOrder returned error: %v", err)
	}
	if !res.OK() {
		t.Fatalf("blocked higher-priority items must not count as open head-of-line blockers; got: %v", res.Violations)
	}
}

func TestEnforcePriorityOrder_CompleteHigherExempt(t *testing.T) {
	items := []PlanItem{
		{ID: "bli-p1", Status: StatusInProgress, Priority: PriorityHigh, PriorityTier: TierP1},
		{ID: "bli-p0-done", Status: StatusComplete, Priority: PriorityCritical, PriorityTier: TierP0},
	}
	res, err := NewEnforcer(nil).EnforceOrder(items)
	if err != nil {
		t.Fatalf("EnforceOrder returned error: %v", err)
	}
	if !res.OK() {
		t.Fatalf("complete items must not violate ordering; got: %v", res.Violations)
	}
}

func TestEnforcePriorityOrder_SameTierInProgressOK(t *testing.T) {
	items := []PlanItem{
		{ID: "a", Status: StatusInProgress, Priority: PriorityHigh, PriorityTier: TierP1},
		{ID: "b", Status: StatusPlanned, Priority: PriorityHigh, PriorityTier: TierP1},
	}
	res, err := NewEnforcer(nil).EnforceOrder(items)
	if err != nil {
		t.Fatalf("EnforceOrder returned error: %v", err)
	}
	if !res.OK() {
		t.Fatalf("same-priority siblings may progress concurrently; got: %v", res.Violations)
	}
}

func TestEnforcePriorityOrder_PairingViolation(t *testing.T) {
	items := []PlanItem{
		{ID: "bli-bad", Status: StatusInProgress, Priority: PriorityCritical, PriorityTier: TierP3},
	}
	res, err := NewEnforcer(nil).EnforceOrder(items)
	if err != nil {
		t.Fatalf("EnforceOrder returned error: %v", err)
	}
	if res.OK() {
		t.Fatal("expected pairing violation for critical/P3, got none")
	}
	if res.Violations[0].Code != CodePriorityPairing {
		t.Fatalf("wrong violation code %q, want %q", res.Violations[0].Code, CodePriorityPairing)
	}
}

func TestEnforcePriorityOrder_MissingPriorityOnExecutionFacing(t *testing.T) {
	items := []PlanItem{
		{ID: "bli-nopri", Status: StatusActive, Priority: "", PriorityTier: ""},
	}
	res, err := NewEnforcer(nil).EnforceOrder(items)
	if err != nil {
		t.Fatalf("EnforceOrder returned error: %v", err)
	}
	if res.OK() {
		t.Fatal("expected missing-priority violation for execution-facing item, got none")
	}
	if res.Violations[0].Code != CodeMissingPriority {
		t.Fatalf("wrong violation code %q, want %q", res.Violations[0].Code, CodeMissingPriority)
	}
}

func TestPriorityRank(t *testing.T) {
	got := PriorityRank(PriorityCritical)
	if got != 0 {
		t.Fatalf("critical must rank 0, got %d", got)
	}
	if PriorityRank(PriorityLow) != 3 {
		t.Fatalf("low must rank 3, got %d", PriorityRank(PriorityLow))
	}
	if PriorityRank("") >= 0 {
		t.Fatalf("unknown priority must rank below zero (invalid), got %d", PriorityRank(""))
	}
}

func TestTierRank(t *testing.T) {
	if TierRank(TierP0) != 0 {
		t.Fatalf("P0 must rank 0, got %d", TierRank(TierP0))
	}
	if TierRank(TierP3) != 3 {
		t.Fatalf("P3 must rank 3, got %d", TierRank(TierP3))
	}
	if TierRank("P9") >= 0 {
		t.Fatalf("unknown tier must rank below zero, got %d", TierRank("P9"))
	}
}

func TestPaired(t *testing.T) {
	if !Paired(PriorityCritical, TierP0) {
		t.Fatal("critical/P0 must be paired")
	}
	if !Paired(PriorityMedium, TierP2) {
		t.Fatal("medium/P2 must be paired")
	}
	if Paired(PriorityCritical, TierP3) {
		t.Fatal("critical/P3 must not be paired")
	}
}
