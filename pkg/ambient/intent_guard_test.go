package ambient

import (
	"errors"
	"math"
	"sync/atomic"
	"testing"
)

// Traceability: BLI-1793578884304291000-a921fdea, CRIT-1793578818089397000-f11a5a3e
// Negative Boundary: corrupt or unanchored intent signals fail-closed without mutating kernel state.

func validIntentSignal() IntentSignal {
	return IntentSignal{
		ID:         "INT-seed-001",
		Intent:     "hydrate priority plan stream",
		Anchor:     "PRI-1793578753204459000-99585380",
		Confidence: 0.91,
		Source:     "ambient.middle_brain",
	}
}

func TestValidateIntentSignal_AcceptsAnchoredWellFormed(t *testing.T) {
	t.Parallel()
	if err := ValidateIntentSignal(validIntentSignal()); err != nil {
		t.Fatalf("expected valid intent signal, got %v", err)
	}
}

func TestValidateIntentSignal_FailClosedCorruptAndUnanchored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mut  func(*IntentSignal)
		want error
	}{
		{
			name: "missing id",
			mut:  func(s *IntentSignal) { s.ID = "" },
			want: ErrIntentSignalIDRequired,
		},
		{
			name: "missing intent text",
			mut:  func(s *IntentSignal) { s.Intent = "   " },
			want: ErrIntentSignalIntentRequired,
		},
		{
			name: "unanchored",
			mut:  func(s *IntentSignal) { s.Anchor = "" },
			want: ErrIntentSignalUnanchored,
		},
		{
			name: "whitespace-only anchor",
			mut:  func(s *IntentSignal) { s.Anchor = " \t" },
			want: ErrIntentSignalUnanchored,
		},
		{
			name: "confidence below zero",
			mut:  func(s *IntentSignal) { s.Confidence = -0.01 },
			want: ErrIntentSignalCorruptConfidence,
		},
		{
			name: "confidence above one",
			mut:  func(s *IntentSignal) { s.Confidence = 1.01 },
			want: ErrIntentSignalCorruptConfidence,
		},
		{
			name: "confidence NaN",
			mut:  func(s *IntentSignal) { s.Confidence = math.NaN() },
			want: ErrIntentSignalCorruptConfidence,
		},
		{
			name: "confidence +Inf",
			mut:  func(s *IntentSignal) { s.Confidence = math.Inf(1) },
			want: ErrIntentSignalCorruptConfidence,
		},
		{
			name: "corrupt control bytes in intent",
			mut:  func(s *IntentSignal) { s.Intent = "hydrate\x00plan" },
			want: ErrIntentSignalCorruptPayload,
		},
		{
			name: "missing source",
			mut:  func(s *IntentSignal) { s.Source = "" },
			want: ErrIntentSignalSourceRequired,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sig := validIntentSignal()
			tc.mut(&sig)
			err := ValidateIntentSignal(sig)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestAdmitIntentSignal_RejectDoesNotMutateKernel(t *testing.T) {
	t.Parallel()

	var mutations atomic.Int64
	mutate := func(IntentSignal) error {
		mutations.Add(1)
		return nil
	}

	bad := validIntentSignal()
	bad.Anchor = ""
	if err := AdmitIntentSignal(bad, mutate); !errors.Is(err, ErrIntentSignalUnanchored) {
		t.Fatalf("expected ErrIntentSignalUnanchored, got %v", err)
	}
	if got := mutations.Load(); got != 0 {
		t.Fatalf("reject must not mutate kernel state; mutations=%d", got)
	}

	good := validIntentSignal()
	if err := AdmitIntentSignal(good, mutate); err != nil {
		t.Fatalf("admit valid signal: %v", err)
	}
	if got := mutations.Load(); got != 1 {
		t.Fatalf("expected exactly one mutation after admit, got %d", got)
	}
}

func TestAdmitIntentSignal_NilMutatorFailClosed(t *testing.T) {
	t.Parallel()
	if err := AdmitIntentSignal(validIntentSignal(), nil); !errors.Is(err, ErrIntentSignalMutatorRequired) {
		t.Fatalf("expected ErrIntentSignalMutatorRequired, got %v", err)
	}
}
