// BLI-STARTER-COMMUNITY-031 / PRI-STARTER-COMMUNITY-031 coverage elevation
package wsobs

import "testing"

func TestMetricSet_AddNilMap(t *testing.T) {
	t.Parallel()
	ms := &MetricSet{ID: "x"}
	if !ms.Add("k", 1, nil) {
		t.Fatal("nil map")
	}
}

func TestHeuristic_NilSetAndTypes(t *testing.T) {
	t.Parallel()
	h := &Heuristic{Name: "h", Field: "n", Min: 0, Max: 10, HardFail: true}
	if err := h.Validate(nil); err == nil {
		t.Fatal("nil set hard fail")
	}
	soft := &Heuristic{Name: "h", Field: "n", Min: 0, Max: 10, HardFail: false}
	if err := soft.Validate(nil); err != nil {
		t.Fatal(err)
	}
	ms := NewMetricSet("id", "l")
	ms.Add("n", float32(3), nil)
	if err := h.Validate(ms); err != nil {
		t.Fatal(err)
	}
	ms2 := NewMetricSet("id2", "l")
	ms2.Add("n", "nope", nil)
	if err := h.Validate(ms2); err == nil {
		t.Fatal("unsupported type")
	}
	if err := soft.Validate(ms2); err != nil {
		t.Fatal("warn-only unsupported")
	}
	ms3 := NewMetricSet("id3", "l")
	ms3.Add("n", uint64(4), nil)
	if err := h.Validate(ms3); err != nil {
		t.Fatal(err)
	}
}

func TestMetricError_Error(t *testing.T) {
	t.Parallel()
	e := MetricError{Message: "x", Code: 7}
	if e.Error() == "" {
		t.Fatal("empty")
	}
}
