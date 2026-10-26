package wsobs

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestMetricItem_JSONSerialization verifies JSON marshaling and unmarshaling of MetricItem.
func TestMetricItem_JSONSerialization(t *testing.T) {
	t.Parallel()

	item := MetricItem{
		Value: 42.5,
		Tags:  []string{"env:test", "tier:core"},
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var decoded MetricItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if decoded.Value != 42.5 {
		t.Errorf("expected value 42.5, got %v", decoded.Value)
	}
	if len(decoded.Tags) != 2 || decoded.Tags[0] != "env:test" || decoded.Tags[1] != "tier:core" {
		t.Errorf("unexpected decoded tags: %v", decoded.Tags)
	}
}

// TestMetricSet_NewAndAddEdgeCases covers NewMetricSet, Add on nil map,
// adding new items, and attempting duplicate additions.
func TestMetricSet_NewAndAddEdgeCases(t *testing.T) {
	t.Parallel()

	// 1. NewMetricSet initializes non-nil empty map
	ms := NewMetricSet("set-alpha", "Alpha Metric Set")
	if ms == nil {
		t.Fatal("expected non-nil MetricSet")
	}
	if ms.ID != "set-alpha" || ms.Label != "Alpha Metric Set" {
		t.Fatalf("unexpected id or label: id=%s label=%s", ms.ID, ms.Label)
	}
	if ms.Metrics == nil || len(ms.Metrics) != 0 {
		t.Fatalf("expected empty non-nil metrics map, got len=%d", len(ms.Metrics))
	}

	// 2. Add with nil map on manually constructed struct
	manualSet := &MetricSet{ID: "manual", Label: "Manual Set"}
	if manualSet.Metrics != nil {
		t.Fatal("expected nil Metrics initially")
	}
	added := manualSet.Add("goroutines", 128, []string{"runtime"})
	if !added {
		t.Error("expected Add on nil map to return true")
	}
	if manualSet.Metrics == nil {
		t.Fatal("expected Add to initialize nil map")
	}
	if item, exists := manualSet.Metrics["goroutines"]; !exists || item.Value != 128 {
		t.Errorf("expected item goroutines=128, got %v", item)
	}

	// 3. Duplicate key should return false and not overwrite
	secondAdd := manualSet.Add("goroutines", 999, []string{"new_tag"})
	if secondAdd {
		t.Error("expected duplicate Add to return false")
	}
	if item := manualSet.Metrics["goroutines"]; item.Value != 128 {
		t.Errorf("expected value to remain 128, got %v", item.Value)
	}

	// 4. JSON roundtrip of MetricSet
	jsonData, err := json.Marshal(manualSet)
	if err != nil {
		t.Fatalf("failed to marshal MetricSet: %v", err)
	}
	var unmarshaledSet MetricSet
	if err := json.Unmarshal(jsonData, &unmarshaledSet); err != nil {
		t.Fatalf("failed to unmarshal MetricSet: %v", err)
	}
	if unmarshaledSet.ID != "manual" || unmarshaledSet.Label != "Manual Set" {
		t.Errorf("unmarshaled mismatch: %+v", unmarshaledSet)
	}
}

// TestMetricError_Formatting tests formatting and code of MetricError.
func TestMetricError_Formatting(t *testing.T) {
	t.Parallel()

	err1 := MetricError{Message: "observation timeout", Code: 408}
	expected := "metric error: observation timeout (code 408)"
	if err1.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err1.Error())
	}

	// JSON serialization
	b, err := json.Marshal(err1)
	if err != nil {
		t.Fatalf("failed to marshal MetricError: %v", err)
	}
	var err2 MetricError
	if err := json.Unmarshal(b, &err2); err != nil {
		t.Fatalf("failed to unmarshal MetricError: %v", err)
	}
	if err2 != err1 {
		t.Errorf("mismatch after json roundtrip: expected %+v, got %+v", err1, err2)
	}
}

// TestMetricSetResult_Lifecycle tests Valid, AddError, and multi-error scenarios.
func TestMetricSetResult_Lifecycle(t *testing.T) {
	t.Parallel()

	res := &MetricSetResult{TotalObserved: 0}
	if !res.Valid() {
		t.Error("expected empty MetricSetResult to be valid")
	}

	res.AddError("disk full", 507)
	if res.Valid() {
		t.Error("expected MetricSetResult with error to be invalid")
	}
	if len(res.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(res.Errors))
	}
	if res.Errors[0].Message != "disk full" || res.Errors[0].Code != 507 {
		t.Errorf("unexpected error payload: %+v", res.Errors[0])
	}

	res.AddError("checksum mismatch", 400)
	if res.Valid() {
		t.Error("expected MetricSetResult with 2 errors to be invalid")
	}
	if len(res.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(res.Errors))
	}

	// JSON serialization
	res.TotalObserved = 10
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("failed to marshal MetricSetResult: %v", err)
	}
	var res2 MetricSetResult
	if err := json.Unmarshal(data, &res2); err != nil {
		t.Fatalf("failed to unmarshal MetricSetResult: %v", err)
	}
	if res2.TotalObserved != 10 || len(res2.Errors) != 2 {
		t.Errorf("unexpected unmarshaled result: %+v", res2)
	}
}

// TestHeuristicViolation_ErrorFormatting covers string generation of HeuristicViolation.
func TestHeuristicViolation_ErrorFormatting(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		violation HeuristicViolation
		contains  string
	}{
		{
			name: "missing field",
			violation: HeuristicViolation{
				Reason: "missing_field",
				Name:   "cpu_usage_check",
				Field:  "cpu_percent",
			},
			contains: `heuristic violation "cpu_usage_check" on field "cpu_percent": reason=missing_field tried=<nil>`,
		},
		{
			name: "below minimum",
			violation: HeuristicViolation{
				Reason: "below_minimum",
				Name:   "thread_count",
				Field:  "threads",
				Tried:  2,
				Lowest: 4,
			},
			contains: `heuristic violation "thread_count" on field "threads": reason=below_minimum tried=2`,
		},
		{
			name: "above maximum",
			violation: HeuristicViolation{
				Reason:  "above_maximum",
				Name:    "memory_cap",
				Field:   "memory_mb",
				Tried:   10240,
				Highest: 8192,
			},
			contains: `heuristic violation "memory_cap" on field "memory_mb": reason=above_maximum tried=10240`,
		},
		{
			name: "unsupported type",
			violation: HeuristicViolation{
				Reason: "unsupported_type",
				Name:   "numeric_check",
				Field:  "status_text",
				Tried:  "healthy",
			},
			contains: `heuristic violation "numeric_check" on field "status_text": reason=unsupported_type tried=healthy`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.violation.Error()
			if got != tc.contains {
				t.Errorf("expected error %q, got %q", tc.contains, got)
			}
		})
	}
}

// TestHeuristic_Validate_NilAndMissingFieldScenarios covers all nil MetricSet,
// nil map, and missing field combinations with HardFail true and false.
func TestHeuristic_Validate_NilAndMissingFieldScenarios(t *testing.T) {
	t.Parallel()

	hHard := &Heuristic{Name: "h_mem", Field: "mem_usage", Min: 10, Max: 100, HardFail: true}
	hSoft := &Heuristic{Name: "h_mem", Field: "mem_usage", Min: 10, Max: 100, HardFail: false}

	// 1. Nil MetricSet
	errNilHard := hHard.Validate(nil)
	if errNilHard == nil {
		t.Fatal("expected error on nil MetricSet with HardFail=true")
	}
	if v, ok := errNilHard.(*HeuristicViolation); !ok || v.Reason != "missing_field" {
		t.Errorf("expected missing_field HeuristicViolation, got %v", errNilHard)
	}

	errNilSoft := hSoft.Validate(nil)
	if errNilSoft != nil {
		t.Fatalf("expected nil error on nil MetricSet with HardFail=false, got %v", errNilSoft)
	}

	// 2. Non-nil MetricSet with nil Metrics map
	nilMapSet := &MetricSet{ID: "s1", Label: "Label1", Metrics: nil}
	errNilMapHard := hHard.Validate(nilMapSet)
	if errNilMapHard == nil {
		t.Fatal("expected error on nil Metrics map with HardFail=true")
	}
	if v, ok := errNilMapHard.(*HeuristicViolation); !ok || v.Reason != "missing_field" {
		t.Errorf("expected missing_field violation, got %v", errNilMapHard)
	}

	errNilMapSoft := hSoft.Validate(nilMapSet)
	if errNilMapSoft != nil {
		t.Fatalf("expected nil error on nil Metrics map with HardFail=false, got %v", errNilMapSoft)
	}

	// 3. Non-nil Metrics map, missing target field
	emptyMapSet := NewMetricSet("s2", "Label2")
	errMissingHard := hHard.Validate(emptyMapSet)
	if errMissingHard == nil {
		t.Fatal("expected error on missing field with HardFail=true")
	}
	if v, ok := errMissingHard.(*HeuristicViolation); !ok || v.Reason != "missing_field" || v.Field != "mem_usage" {
		t.Errorf("unexpected violation: %v", errMissingHard)
	}

	errMissingSoft := hSoft.Validate(emptyMapSet)
	if errMissingSoft != nil {
		t.Fatalf("expected nil error on missing field with HardFail=false, got %v", errMissingSoft)
	}
}

// TestHeuristic_Validate_NumericTypes covers all supported numeric types:
// int, int32, int64, float64, float32, uint, uint32, uint64.
func TestHeuristic_Validate_NumericTypes(t *testing.T) {
	t.Parallel()

	h := &Heuristic{
		Name:     "range_50_to_100",
		Field:    "metric_val",
		Min:      50.0,
		Max:      100.0,
		HardFail: true,
	}

	numericFixtures := []struct {
		typeName string
		value    interface{}
	}{
		{"int", int(75)},
		{"int32", int32(75)},
		{"int64", int64(75)},
		{"float64", float64(75.5)},
		{"float32", float32(75.5)},
		{"uint", uint(75)},
		{"uint32", uint32(75)},
		{"uint64", uint64(75)},
	}

	for _, fix := range numericFixtures {
		t.Run(fmt.Sprintf("Type_%s_WithinRange", fix.typeName), func(t *testing.T) {
			ms := NewMetricSet("num_set", "Numeric Test Set")
			ms.Add("metric_val", fix.value, nil)

			if err := h.Validate(ms); err != nil {
				t.Fatalf("expected value of type %s within range to pass, got: %v", fix.typeName, err)
			}
		})
	}
}

// TestHeuristic_Validate_BoundaryConditions covers below minimum, above maximum,
// and exact boundary values (equal to Min, equal to Max).
func TestHeuristic_Validate_BoundaryConditions(t *testing.T) {
	t.Parallel()

	h := &Heuristic{
		Name:     "bound_check",
		Field:    "score",
		Min:      10.0,
		Max:      90.0,
		HardFail: true,
	}

	// 1. Below minimum
	msBelow := NewMetricSet("b1", "Below")
	msBelow.Add("score", 9.99, nil)
	errBelow := h.Validate(msBelow)
	if errBelow == nil {
		t.Fatal("expected below_minimum violation")
	}
	if v, ok := errBelow.(*HeuristicViolation); !ok || v.Reason != "below_minimum" || v.Lowest != 10.0 {
		t.Errorf("unexpected below error: %v", errBelow)
	}

	// 2. Above maximum
	msAbove := NewMetricSet("b2", "Above")
	msAbove.Add("score", 90.01, nil)
	errAbove := h.Validate(msAbove)
	if errAbove == nil {
		t.Fatal("expected above_maximum violation")
	}
	if v, ok := errAbove.(*HeuristicViolation); !ok || v.Reason != "above_maximum" || v.Highest != 90.0 {
		t.Errorf("unexpected above error: %v", errAbove)
	}

	// 3. Exact min boundary (inclusive)
	msMin := NewMetricSet("b3", "Exact Min")
	msMin.Add("score", 10.0, nil)
	if err := h.Validate(msMin); err != nil {
		t.Errorf("exact Min boundary should pass, got: %v", err)
	}

	// 4. Exact max boundary (inclusive)
	msMax := NewMetricSet("b4", "Exact Max")
	msMax.Add("score", 90.0, nil)
	if err := h.Validate(msMax); err != nil {
		t.Errorf("exact Max boundary should pass, got: %v", err)
	}
}

// TestHeuristic_Validate_UnsupportedTypeScenarios covers non-numeric data types
// with HardFail=true and HardFail=false.
func TestHeuristic_Validate_UnsupportedTypeScenarios(t *testing.T) {
	t.Parallel()

	hHard := &Heuristic{Name: "type_sentinel", Field: "payload", Min: 1, Max: 10, HardFail: true}
	hSoft := &Heuristic{Name: "type_sentinel", Field: "payload", Min: 1, Max: 10, HardFail: false}

	unsupportedValues := []interface{}{
		"not_a_number",
		true,
		[]int{1, 2, 3},
		map[string]int{"val": 5},
		struct{ X int }{X: 10},
	}

	for i, val := range unsupportedValues {
		t.Run(fmt.Sprintf("UnsupportedVal_%d", i), func(t *testing.T) {
			ms := NewMetricSet("unsup", "Unsupported")
			ms.Add("payload", val, nil)

			// HardFail=true should produce unsupported_type violation
			errHard := hHard.Validate(ms)
			if errHard == nil {
				t.Fatalf("expected unsupported_type error for value %v with HardFail=true", val)
			}
			v, ok := errHard.(*HeuristicViolation)
			if !ok || v.Reason != "unsupported_type" || !reflect.DeepEqual(v.Tried, val) {
				t.Errorf("unexpected violation structure: %+v", errHard)
			}

			// HardFail=false should return nil
			errSoft := hSoft.Validate(ms)
			if errSoft != nil {
				t.Errorf("expected nil error for value %v with HardFail=false, got: %v", val, errSoft)
			}
		})
	}
}

// TestObservePackages_AllInputs covers ObservePackages with empty, populated, or nil slices.
func TestObservePackages_AllInputs(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		nil,
		{},
		{"pkg/wsobs"},
		{"pkg/kernel", "pkg/storage", "pkg/validation"},
	}

	for _, pkgs := range cases {
		res := ObservePackages(pkgs)
		if res == nil {
			t.Fatal("expected non-nil MetricSetResult")
		}
		if res.TotalObserved != 1 {
			t.Errorf("expected TotalObserved=1, got %d", res.TotalObserved)
		}
		if !res.Valid() {
			t.Errorf("expected clean result to be valid, got errors: %v", res.Errors)
		}
	}
}

// TestWorkspaceHeuristics_Lifecycle covers NewWorkspaceHeuristics and custom configurations.
func TestWorkspaceHeuristics_Lifecycle(t *testing.T) {
	t.Parallel()

	wh := NewWorkspaceHeuristics()
	if wh == nil {
		t.Fatal("expected non-nil WorkspaceHeuristics")
	}
	if wh.PackageThreshold != 1 {
		t.Errorf("expected PackageThreshold=1, got %d", wh.PackageThreshold)
	}
	if wh.FailureTolerance != 0 {
		t.Errorf("expected FailureTolerance=0, got %d", wh.FailureTolerance)
	}

	// Custom mutation and JSON serialization
	wh.PackageThreshold = 10
	wh.FailureTolerance = 2

	data, err := json.Marshal(wh)
	if err != nil {
		t.Fatalf("failed to marshal WorkspaceHeuristics: %v", err)
	}

	var wh2 WorkspaceHeuristics
	if err := json.Unmarshal(data, &wh2); err != nil {
		t.Fatalf("failed to unmarshal WorkspaceHeuristics: %v", err)
	}
	if wh2.PackageThreshold != 10 || wh2.FailureTolerance != 2 {
		t.Errorf("unexpected decoded heuristics: %+v", wh2)
	}
}

// TestMetricSet_ConcurrentOperations verifies concurrency safety for isolated sets
// and read-evaluate concurrency across multiple heuristics.
func TestMetricSet_ConcurrentOperations(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	setsCount := 30
	sets := make([]*MetricSet, setsCount)

	for i := 0; i < setsCount; i++ {
		sets[i] = NewMetricSet(fmt.Sprintf("set-%d", i), fmt.Sprintf("Label %d", i))
	}

	// Concurrent writes across distinct sets
	for i := 0; i < setsCount; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("test_wsobs_concurrent_writer", fmt.Sprintf("writer-%d", idx)).StartSimple(func() {
			defer wg.Done()
			s := sets[idx]
			s.Add("metric_a", idx*10, []string{"dim:idx"})
			s.Add("metric_b", float64(idx)+0.5, []string{"dim:float"})
		})
	}
	wg.Wait()

	// Concurrent validation runs across distinct sets
	heur := &Heuristic{
		Name:     "concurrent_eval",
		Field:    "metric_a",
		Min:      0,
		Max:      1000,
		HardFail: true,
	}

	for i := 0; i < setsCount; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("test_wsobs_concurrent_validator", fmt.Sprintf("validator-%d", idx)).StartSimple(func() {
			defer wg.Done()
			if err := heur.Validate(sets[idx]); err != nil {
				t.Errorf("set %d validation failed unexpectedly: %v", idx, err)
			}
		})
	}
	wg.Wait()
}
