package scheduler

import (
	"testing"
)

func TestEvaluationSurfaceIDFromMaps(t *testing.T) {
	t.Parallel()
	if got := EvaluationSurfaceIDFromMaps(nil, nil); got != EvaluationSurfaceSchedulerTestBundleHealthJSONL {
		t.Fatalf("default: got %q", got)
	}
	after := map[string]any{"evaluation_surface": EvaluationSurfaceCEFDiamondScorecard}
	if got := EvaluationSurfaceIDFromMaps(after, nil); got != EvaluationSurfaceCEFDiamondScorecard {
		t.Fatalf("after: got %q", got)
	}
	before := map[string]any{"evaluation_surface_id": EvaluationSurfaceCEFDiamondScorecard}
	if got := EvaluationSurfaceIDFromMaps(nil, before); got != EvaluationSurfaceCEFDiamondScorecard {
		t.Fatalf("before: got %q", got)
	}
}

func TestEvaluationSurfaceRegistry(t *testing.T) {
	t.Parallel()
	a := getEvaluationSurfaceAdapter(EvaluationSurfaceCEFDiamondScorecard)
	if a == nil {
		t.Fatalf("expected CEF diamond adapter to be registered")
	}
	b := getEvaluationSurfaceAdapter(EvaluationSurfaceSchedulerTestBundleHealthJSONL)
	if b == nil {
		t.Fatalf("expected test bundle adapter to be registered")
	}
}
