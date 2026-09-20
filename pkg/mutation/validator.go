package mutation

import (
	"context"
	"fmt"
)

// HilGater defines the interface for prompting a Human-In-The-Loop gate
type HilGater interface {
	HilGate(ctx context.Context, taskID string, mut *Mutation) error
}

// Validator routes mutations based on their safety class.
type Validator struct {
	Gate HilGater
}

func NewValidator(gate HilGater) *Validator {
	return &Validator{Gate: gate}
}

// ValidateAndRoute checks if a mutation is safe to apply or requires a HIL gate.
func (v *Validator) ValidateAndRoute(ctx context.Context, taskID string, mut *Mutation) error {
	if mut.SafetyClass == SafetyDestructive || mut.SafetyClass == SafetyHilRequired {
		if v.Gate == nil {
			return fmt.Errorf("mutation requires HIL gate but none configured")
		}
		if err := v.Gate.HilGate(ctx, taskID, mut); err != nil {
			return err
		}
	}
	return nil
}
