package wsobs

import (
	"testing"
)

func TestNewMetricSet_CreatesEmptyMap(t *testing.T) {
	ms := NewMetricSet("id-1", "label-1")
	if ms.Metrics == nil {
		t.Fatal("expected non-nil metrics map")
	}
	if len(ms.Metrics) != 0 {
		t.Fatalf("expected empty metrics, got %d", len(ms.Metrics))
	}
}

func TestMetricSet_Add_DuplicatesReturnsFalse(t *testing.T) {
	ms := NewMetricSet("id-2", "label-2")
	ok1 := ms.Add("pkg-count", 5, []string{"a"})
	if !ok1 {
		t.Fatal("first add should succeed")
	}
	ok2 := ms.Add("pkg-count", 6, []string{"b"})
	if ok2 {
		t.Fatal("second add with same key should return false")
	}
}

func TestMetricSet_Add_Single(t *testing.T) {
	ms := NewMetricSet("id-3", "label-3")
	ok := ms.Add("pkg-count", 5, []string{"pkg1"})
	if !ok {
		t.Fatal("expected first add to succeed")
	}
	v, ok := ms.Metrics["pkg-count"]
	if !ok {
		t.Fatal("metric should be present in set")
	}
	if v.Value != 5 {
		t.Fatalf("expected value 5, got %d", v.Value)
	}
}

func TestMetricSetResult_Valid(t *testing.T) {
	r := &MetricSetResult{}
	if !r.Valid() {
		t.Fatal("expected Valid to be true when no failures")
	}
}

func TestMetricSetResult_Invalid(t *testing.T) {
	r := &MetricSetResult{}
	r.AddError("something failed", 2001)
	if r.Valid() {
		t.Fatal("expected Valid to be false after adding failure")
	}
}

func TestHeuristicValidate_MissingFieldHardFail(t *testing.T) {
	ms := NewMetricSet("id-4", "label-4")
	h := &Heuristic{Name: "h1", Field: "_missing_", Min: 0, Max: 10, HardFail: true}
	err := h.Validate(ms)
	if err == nil {
		t.Fatal("expected error for missing metric with hard fail")
	}
	_, isViolation := err.(*HeuristicViolation)
	if !isViolation {
		t.Fatalf("expected HeuristicViolation type, got %T", err)
	}
}

func TestHeuristicValidate_MissingFieldWarnOnly(t *testing.T) {
	ms := NewMetricSet("id-5", "label-5")
	h := &Heuristic{Name: "h1", Field: "_missing_", Min: 0, Max: 10, HardFail: false}
	err := h.Validate(ms)
	if err != nil {
		t.Fatalf("expected no error for missing metric in warn-only heuristic, got: %v", err)
	}
}

func TestHeuristicValidate_BelowMinimumHardFail(t *testing.T) {
	ms := NewMetricSet("id-6", "label-6")
	ms.Add("_c_", -5, nil)
	h := &Heuristic{Name: "h2", Field: "_c_", Min: 0, Max: 100, HardFail: true}
	err := h.Validate(ms)
	if err == nil {
		t.Fatal("expected error for below minimum")
	}
	_, isViolation := err.(*HeuristicViolation)
	if !isViolation {
		t.Fatalf("expected HeuristicViolation type, got %T", err)
	}
}

func TestHeuristicValidate_AboveMaximumHardFail(t *testing.T) {
	ms := NewMetricSet("id-7", "label-7")
	ms.Add("_c_", 200, nil)
	h := &Heuristic{Name: "h3", Field: "_c_", Min: 0, Max: 100, HardFail: true}
	err := h.Validate(ms)
	if err == nil {
		t.Fatal("expected error for exceeded maximum")
	}
	_, isViolation := err.(*HeuristicViolation)
	if !isViolation {
		t.Fatalf("expected HeuristicViolation type, got %T", err)
	}
}

func TestHeuristicValidate_WithinRangeReturnsNil(t *testing.T) {
	ms := NewMetricSet("id-8", "label-8")
	ms.Add("_c_", 50, nil)
	h := &Heuristic{Name: "h4", Field: "_c_", Min: 0, Max: 100, HardFail: true}
	err := h.Validate(ms)
	if err != nil {
		t.Fatalf("expected no error for value within range, got: %v", err)
	}
}

func TestHeuristicViolation_Error(t *testing.T) {
	v := &HeuristicViolation{Reason: "below_minimum", Name: "h5", Field: "_c_", Tried: -10, Lowest: 0}
	msg := v.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestObservationPackages_PackagesNilMap(t *testing.T) {
	result := ObservePackages([]string{})
	if result.TotalObserved != 1 {
		t.Fatalf("expected 1 set observed, got %d", result.TotalObserved)
	}
}

func TestHeuristics_New(t *testing.T) {
	w := NewWorkspaceHeuristics()
	if w.PackageThreshold != 1 {
		t.Fatalf("expected PackageThreshold 1, got %d", w.PackageThreshold)
	}
	if w.FailureTolerance != 0 {
		t.Fatalf("expected FailureTolerance 0, got %d", w.FailureTolerance)
	}
}
