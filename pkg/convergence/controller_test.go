package convergence

import (
	"context"
	"testing"
)

type mockConvergenceController struct{}

func (m *mockConvergenceController) Reconcile(ctx context.Context, planID string) error {
	return nil
}

func (m *mockConvergenceController) DetectDrift(ctx context.Context, planID string) ([]DriftReport, error) {
	return []DriftReport{{BacklogItemID: "test-bli", Severity: "low", Message: "none"}}, nil
}

func TestConvergenceControllerInterface(t *testing.T) {
	var _ ConvergenceController = &mockConvergenceController{}
}
