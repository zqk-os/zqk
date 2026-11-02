package ambient

import (
	"math"
	"strings"
	"unicode"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Negative Boundary Guardrails for Intent Signals & Verification Fail-Closes —
// corrupt or unanchored intent seeds are rejected without mutating kernel state.

var (
	// ErrIntentSignalIDRequired is returned when an intent signal lacks a stable id.
	ErrIntentSignalIDRequired = errfmt.Errorf("ambient intent signal id is required")
	// ErrIntentSignalIntentRequired is returned when intent text is empty/whitespace.
	ErrIntentSignalIntentRequired = errfmt.Errorf("ambient intent signal intent is required")
	// ErrIntentSignalUnanchored is returned when the signal has no kernel/stream anchor.
	ErrIntentSignalUnanchored = errfmt.Errorf("ambient intent signal is unanchored")
	// ErrIntentSignalCorruptConfidence is returned when confidence is outside [0,1] or non-finite.
	ErrIntentSignalCorruptConfidence = errfmt.Errorf("ambient intent signal confidence is corrupt")
	// ErrIntentSignalCorruptPayload is returned when intent text contains control/null bytes.
	ErrIntentSignalCorruptPayload = errfmt.Errorf("ambient intent signal payload is corrupt")
	// ErrIntentSignalSourceRequired is returned when provenance source is missing.
	ErrIntentSignalSourceRequired = errfmt.Errorf("ambient intent signal source is required")
	// ErrIntentSignalMutatorRequired is returned when AdmitIntentSignal is called without a mutator.
	ErrIntentSignalMutatorRequired = errfmt.Errorf("ambient intent signal mutator is required")
)

// IntentSignal is a middle-brain intent seed that must be anchored before kernel mutation.
type IntentSignal struct {
	ID         string
	Intent     string
	Anchor     string
	Confidence float64
	Source     string
}

// IntentSignalMutator applies a verified intent signal to kernel/stream state.
// Implementations must treat invocation as the only permission to mutate.
type IntentSignalMutator func(IntentSignal) error

// ValidateIntentSignal fail-closes on corrupt or unanchored intent signals.
func ValidateIntentSignal(sig IntentSignal) error {
	if strings.TrimSpace(sig.ID) == "" {
		return ErrIntentSignalIDRequired
	}
	intent := strings.TrimSpace(sig.Intent)
	if intent == "" {
		return ErrIntentSignalIntentRequired
	}
	if containsCorruptPayload(sig.Intent) {
		return ErrIntentSignalCorruptPayload
	}
	if strings.TrimSpace(sig.Anchor) == "" {
		return ErrIntentSignalUnanchored
	}
	if math.IsNaN(sig.Confidence) || math.IsInf(sig.Confidence, 0) || sig.Confidence < 0 || sig.Confidence > 1 {
		return ErrIntentSignalCorruptConfidence
	}
	if strings.TrimSpace(sig.Source) == "" {
		return ErrIntentSignalSourceRequired
	}
	return nil
}

// AdmitIntentSignal validates then, only on success, invokes mutator.
// Rejects never call mutator — verification fail-close without kernel mutation.
func AdmitIntentSignal(sig IntentSignal, mutator IntentSignalMutator) error {
	if mutator == nil {
		return ErrIntentSignalMutatorRequired
	}
	if err := ValidateIntentSignal(sig); err != nil {
		return err
	}
	return mutator(sig)
}

func containsCorruptPayload(s string) bool {
	for _, r := range s {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r') {
			return true
		}
	}
	return false
}
