package objects

import (
	"testing"
)

func TestPriorityToTier(t *testing.T) {
	tests := []struct {
		priority string
		wantTier string
		wantOk   bool
	}{
		{"critical", "P0", true},
		{"CRITICAL", "P0", true},
		{"Critical", "P0", true},
		{"p0", "P0", true},
		{"P0", "P0", true},
		{"high", "P1", true},
		{"HIGH", "P1", true},
		{"p1", "P1", true},
		{"P1", "P1", true},
		{"medium", "P2", true},
		{"MEDIUM", "P2", true},
		{"p2", "P2", true},
		{"P2", "P2", true},
		{"low", "P3", true},
		{"LOW", "P3", true},
		{"p3", "P3", true},
		{"P3", "P3", true},
		{"invalid", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			got, ok := PriorityToTier(tt.priority)
			if ok != tt.wantOk || got != tt.wantTier {
				t.Errorf("PriorityToTier(%q) = (%q, %v), want (%q, %v)", tt.priority, got, ok, tt.wantTier, tt.wantOk)
			}
		})
	}
}

func TestTierToPriority(t *testing.T) {
	tests := []struct {
		tier         string
		wantPriority string
		wantOk       bool
	}{
		{"P0", "critical", true},
		{"p0", "critical", true},
		{"critical", "critical", true},
		{"CRITICAL", "critical", true},
		{"P1", "high", true},
		{"p1", "high", true},
		{"high", "high", true},
		{"HIGH", "high", true},
		{"P2", "medium", true},
		{"p2", "medium", true},
		{"medium", "medium", true},
		{"MEDIUM", "medium", true},
		{"P3", "low", true},
		{"p3", "low", true},
		{"low", "low", true},
		{"LOW", "low", true},
		{"invalid", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			got, ok := TierToPriority(tt.tier)
			if ok != tt.wantOk || got != tt.wantPriority {
				t.Errorf("TierToPriority(%q) = (%q, %v), want (%q, %v)", tt.tier, got, ok, tt.wantPriority, tt.wantOk)
			}
		})
	}
}

func TestCanonicalPriorityAndTier(t *testing.T) {
	// CanonicalPriority
	if p, ok := CanonicalPriority("p0"); !ok || p != PriorityCritical {
		t.Errorf("CanonicalPriority(p0) = (%q, %v)", p, ok)
	}
	if p, ok := CanonicalPriority("high"); !ok || p != PriorityHigh {
		t.Errorf("CanonicalPriority(high) = (%q, %v)", p, ok)
	}
	if p, ok := CanonicalPriority("medium"); !ok || p != PriorityMedium {
		t.Errorf("CanonicalPriority(medium) = (%q, %v)", p, ok)
	}
	if p, ok := CanonicalPriority("low"); !ok || p != PriorityLow {
		t.Errorf("CanonicalPriority(low) = (%q, %v)", p, ok)
	}
	if _, ok := CanonicalPriority("invalid"); ok {
		t.Error("expected ok=false for invalid priority")
	}

	// CanonicalPriorityTier
	if tier, ok := CanonicalPriorityTier("critical"); !ok || tier != PriorityTierP0 {
		t.Errorf("CanonicalPriorityTier(critical) = (%q, %v)", tier, ok)
	}
	if tier, ok := CanonicalPriorityTier("high"); !ok || tier != PriorityTierP1 {
		t.Errorf("CanonicalPriorityTier(high) = (%q, %v)", tier, ok)
	}
	if tier, ok := CanonicalPriorityTier("medium"); !ok || tier != PriorityTierP2 {
		t.Errorf("CanonicalPriorityTier(medium) = (%q, %v)", tier, ok)
	}
	if tier, ok := CanonicalPriorityTier("low"); !ok || tier != PriorityTierP3 {
		t.Errorf("CanonicalPriorityTier(low) = (%q, %v)", tier, ok)
	}
	if _, ok := CanonicalPriorityTier("invalid"); ok {
		t.Error("expected ok=false for invalid tier")
	}
}

func TestCoerceBacklogItemPriorityAndTier(t *testing.T) {
	// 1. Only priority provided -> sets priority_tier
	obj1 := map[string]any{
		FieldKeyPriority: "high",
	}
	CoerceBacklogItemPriorityAndTier(obj1)
	if obj1[FieldKeyPriority] != "high" || obj1[FieldKeyPriorityTier] != "P1" {
		t.Errorf("obj1: got priority=%v, tier=%v; want high, P1", obj1[FieldKeyPriority], obj1[FieldKeyPriorityTier])
	}

	// 2. Only priority_tier provided -> sets priority
	obj2 := map[string]any{
		FieldKeyPriorityTier: "P0",
	}
	CoerceBacklogItemPriorityAndTier(obj2)
	if obj2[FieldKeyPriority] != "critical" || obj2[FieldKeyPriorityTier] != "P0" {
		t.Errorf("obj2: got priority=%v, tier=%v; want critical, P0", obj2[FieldKeyPriority], obj2[FieldKeyPriorityTier])
	}

	// 3. Both provided with legitimate values -> normalized casing preserved
	obj3 := map[string]any{
		FieldKeyPriority:     "Medium",
		FieldKeyPriorityTier: "p2",
	}
	CoerceBacklogItemPriorityAndTier(obj3)
	if obj3[FieldKeyPriority] != "medium" || obj3[FieldKeyPriorityTier] != "P2" {
		t.Errorf("obj3: got priority=%v, tier=%v; want medium, P2", obj3[FieldKeyPriority], obj3[FieldKeyPriorityTier])
	}

	// 4. Both provided with legitimate values that differ -> preserves both, normalizes casing
	obj4 := map[string]any{
		FieldKeyPriority:     "high",
		FieldKeyPriorityTier: "P2",
	}
	CoerceBacklogItemPriorityAndTier(obj4)
	if obj4[FieldKeyPriority] != "high" || obj4[FieldKeyPriorityTier] != "P2" {
		t.Errorf("obj4: got priority=%v, tier=%v; want high, P2", obj4[FieldKeyPriority], obj4[FieldKeyPriorityTier])
	}

	// 5. Neither provided -> no-op
	obj5 := map[string]any{
		FieldKeyTitle: "Sample BLI",
	}
	CoerceBacklogItemPriorityAndTier(obj5)
	if _, ok := obj5[FieldKeyPriority]; ok {
		t.Errorf("obj5: priority should not be set")
	}
	if _, ok := obj5[FieldKeyPriorityTier]; ok {
		t.Errorf("obj5: priority_tier should not be set")
	}

	// 6. Nil object -> no panic
	CoerceBacklogItemPriorityAndTier(nil)
}

func TestCoerceRequirementPriority(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"critical", "p0"},
		{"Critical", "p0"},
		{"CRITICAL", "p0"},
		{"P0", "p0"},
		{"p0", "p0"},
		{"high", "p1"},
		{"High", "p1"},
		{"P1", "p1"},
		{"p1", "p1"},
		{"medium", "p2"},
		{"Medium", "p2"},
		{"P2", "p2"},
		{"p2", "p2"},
		{"low", "p3"},
		{"Low", "p3"},
		{"P3", "p3"},
		{"p3", "p3"},
	}

	for _, tt := range tests {
		obj := map[string]any{
			FieldKeyPriority: tt.input,
		}
		CoerceRequirementPriority(obj)
		if got := obj[FieldKeyPriority]; got != tt.want {
			t.Errorf("CoerceRequirementPriority(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}

	// Unknown or missing priority preserved/no-op
	objNone := map[string]any{FieldKeyTitle: "Req"}
	CoerceRequirementPriority(objNone)
	if _, ok := objNone[FieldKeyPriority]; ok {
		t.Error("priority should not be set if missing")
	}

	// Nil object -> no panic
	CoerceRequirementPriority(nil)
}

func TestNormalizePriority_Unified(t *testing.T) {
	// 1. Requirement word alias to p-tier
	req := map[string]any{
		FieldKeyKind:     KindRequirement,
		FieldKeyPriority: "medium",
	}
	NormalizePriority(KindRequirement, req)
	if req[FieldKeyPriority] != "p2" {
		t.Errorf("NormalizePriority requirement: got %v, want p2", req[FieldKeyPriority])
	}

	// 2. Backlog item p-tier to word label
	bli := map[string]any{
		FieldKeyKind:     KindBacklogItem,
		FieldKeyPriority: "p2",
	}
	NormalizePriority(KindBacklogItem, bli)
	if bli[FieldKeyPriority] != "medium" || bli[FieldKeyPriorityTier] != "P2" {
		t.Errorf("NormalizePriority bli: got priority=%v tier=%v, want medium, P2", bli[FieldKeyPriority], bli[FieldKeyPriorityTier])
	}

	// 3. Generic kind normalization
	gen := map[string]any{
		FieldKeyPriority: "HIGH",
	}
	NormalizePriority("custom", gen)
	if gen[FieldKeyPriority] != "high" {
		t.Errorf("NormalizePriority generic: got %v, want high", gen[FieldKeyPriority])
	}

	// 4. Nil object -> no panic
	NormalizePriority("", nil)
	CoercePriority("", nil)
}

func TestValidatePriority_ValidationAndBounds(t *testing.T) {
	valid := []string{"critical", "high", "medium", "low", "p0", "p1", "p2", "p3", "P0", "P1", "P2", "P3"}
	for _, v := range valid {
		if err := ValidatePriority(v); err != nil {
			t.Errorf("ValidatePriority(%q) returned unexpected error: %v", v, err)
		}
		if err := ValidatePriorityForKind(KindRequirement, v); err != nil {
			t.Errorf("ValidatePriorityForKind(requirement, %q) returned unexpected error: %v", v, err)
		}
	}

	invalid := []string{"urgent", "p4", "unknown", "blocker", ""}
	for _, inv := range invalid {
		if err := ValidatePriority(inv); err == nil {
			t.Errorf("ValidatePriority(%q) expected error, got nil", inv)
		}
		if err := ValidatePriorityForKind(KindRequirement, inv); err == nil {
			t.Errorf("ValidatePriorityForKind(requirement, %q) expected error, got nil", inv)
		}
	}

	// ValidatePriorityAndTier
	errValid := ValidatePriorityAndTier(map[string]any{
		FieldKeyPriority:     "medium",
		FieldKeyPriorityTier: "P2",
	})
	if errValid != nil {
		t.Errorf("ValidatePriorityAndTier expected nil, got %v", errValid)
	}

	errInvalid := ValidatePriorityAndTier(map[string]any{
		FieldKeyPriority: "urgent",
	})
	if errInvalid == nil {
		t.Errorf("ValidatePriorityAndTier with urgent expected error, got nil")
	}
}
