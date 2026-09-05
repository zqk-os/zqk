package audit

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// Policy defines the interface for an audit policy.
type Policy interface {
	Name() string
	Evaluate(ctx context.Context, record AuditRecord) error
}

// PolicyViolationError is returned when a record violates a policy.
type PolicyViolationError struct {
	PolicyName string
	Reason     string
}

func (e *PolicyViolationError) Error() string {
	return fmt.Sprintf("policy %s violated: %s", e.PolicyName, e.Reason)
}

// PolicyEngine evaluates audit records against registered policies.
type PolicyEngine struct {
	policies []Policy
}

// NewPolicyEngine creates a new PolicyEngine.
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		policies: make([]Policy, 0),
	}
}

// RegisterPolicy adds a policy to the engine.
func (e *PolicyEngine) RegisterPolicy(p Policy) {
	e.policies = append(e.policies, p)
}

// Validate checks a record against all registered policies.
// It returns a PolicyViolationError if any policy is violated.
func (e *PolicyEngine) Validate(ctx context.Context, record AuditRecord) error {
	eg, gCtx := errgroup.WithContext(ctx)
	for _, p := range e.policies {
		p := p // capture loop variable
		eg.Go(func() error {
			return p.Evaluate(gCtx, record)
		})
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- eg.Wait()
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
