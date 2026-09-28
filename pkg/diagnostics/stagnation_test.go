package diagnostics_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/diagnostics"
)

func TestEvaluateStagnation_ActiveProgress(t *testing.T) {
	// Attempt 1 fails on syntax, Attempt 2 fails on unit test (different signature)
	env1 := diagnostics.ExtractFailureEnvelope(1, 1, errors.New("syntax error"), nil, nil, nil)
	env2 := diagnostics.ExtractFailureEnvelope(2, 1, errors.New("unit test failed"), nil, nil, nil)

	policy := diagnostics.DefaultStagnationPolicy()
	assessment := diagnostics.EvaluateStagnation([]diagnostics.FailureEnvelope{env1, env2}, policy)

	if assessment.IsStagnant {
		t.Error("expected non-stagnant assessment when diagnostic signatures differ")
	}
	if assessment.Recommendation != diagnostics.RecommendationContinue {
		t.Errorf("expected recommendation %q, got %q", diagnostics.RecommendationContinue, assessment.Recommendation)
	}
}

func TestEvaluateStagnation_ConsecutiveZeroDelta(t *testing.T) {
	// Attempt 1 and Attempt 2 have identical failures
	env1 := diagnostics.ExtractFailureEnvelope(1, 1, errors.New("connection timeout"), []string{"ping"}, nil, nil)
	env2 := diagnostics.ExtractFailureEnvelope(2, 1, errors.New("connection timeout"), []string{"ping"}, nil, nil)

	policy := diagnostics.DefaultStagnationPolicy()
	assessment := diagnostics.EvaluateStagnation([]diagnostics.FailureEnvelope{env1, env2}, policy)

	if !assessment.IsStagnant {
		t.Error("expected stagnant assessment when consecutive attempts have zero delta")
	}
	if assessment.ConsecutiveZeroDelta != 1 {
		t.Errorf("expected consecutive zero delta 1, got %d", assessment.ConsecutiveZeroDelta)
	}
	if assessment.Recommendation != diagnostics.RecommendationPivot {
		t.Errorf("expected recommendation %q, got %q", diagnostics.RecommendationPivot, assessment.Recommendation)
	}

	err := diagnostics.StagnationError(assessment)
	if err == nil || !strings.Contains(err.Error(), "loop stagnation") {
		t.Errorf("expected stagnation error, got: %v", err)
	}
}

func TestEvaluateStagnation_EscalationOnRepeatedStagnation(t *testing.T) {
	// Attempt 1, 2, and 3 repeat the exact same failure
	env1 := diagnostics.ExtractFailureEnvelope(1, 1, errors.New("crash"), nil, nil, nil)
	env2 := diagnostics.ExtractFailureEnvelope(2, 1, errors.New("crash"), nil, nil, nil)
	env3 := diagnostics.ExtractFailureEnvelope(3, 1, errors.New("crash"), nil, nil, nil)

	policy := diagnostics.DefaultStagnationPolicy()
	assessment := diagnostics.EvaluateStagnation([]diagnostics.FailureEnvelope{env1, env2, env3}, policy)

	if !assessment.IsStagnant {
		t.Error("expected stagnation after 3 identical attempts")
	}
	if !assessment.Escalate {
		t.Error("expected escalation to be true after 3 identical attempts")
	}
	if assessment.Recommendation != diagnostics.RecommendationEscalate {
		t.Errorf("expected recommendation %q, got %q", diagnostics.RecommendationEscalate, assessment.Recommendation)
	}
}

func TestBuildCheckpoints_EntropyTracking(t *testing.T) {
	env1 := diagnostics.ExtractFailureEnvelope(1, 1, errors.New("first"), nil, nil, nil)
	env2 := diagnostics.ExtractFailureEnvelope(2, 1, errors.New("first"), nil, nil, nil)
	env3 := diagnostics.ExtractFailureEnvelope(3, 1, errors.New("second"), nil, nil, nil)

	checkpoints := diagnostics.BuildCheckpoints([]diagnostics.FailureEnvelope{env1, env2, env3})
	if len(checkpoints) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(checkpoints))
	}

	if !checkpoints[0].EntropyDelta {
		t.Error("first checkpoint should have entropy delta true")
	}
	if checkpoints[1].EntropyDelta {
		t.Error("second checkpoint with identical signature should have entropy delta false")
	}
	if !checkpoints[2].EntropyDelta {
		t.Error("third checkpoint with new signature should have entropy delta true")
	}
}
