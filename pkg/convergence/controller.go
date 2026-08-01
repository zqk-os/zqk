package convergence

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// DriftReport represents a gap between a planned item and its execution status.
type DriftReport struct {
	BacklogItemID string
	Severity      string
	Message       string
}

// ConvergenceController interface for reconciling roadmap state.
type ConvergenceController interface {
	Reconcile(ctx context.Context, planID string) error
	DetectDrift(ctx context.Context, planID string) ([]DriftReport, error)
}

// Manager implementation for convergence.
type Manager struct {
	storage storage.ObjectStorageProvider
}

func NewManager(s storage.ObjectStorageProvider) *Manager {
	return &Manager{storage: s}
}

func (m *Manager) Reconcile(ctx context.Context, planID string) error {
	// Reconcile logic will be triggered here.
	return nil
}

func (m *Manager) DetectDrift(ctx context.Context, planID string) ([]DriftReport, error) {
	// Query the storage for items linked to this plan
	filter := storage.ListFilter{
		Filters: map[string]any{objects.FieldKeyPriorityPlanRef: planID},
	}

	results, err := m.storage.List(ctx, nil, nil, filter)
	if err != nil {
		return nil, err
	}

	var drifts []DriftReport
	for _, obj := range results.Objects {
		// Basic drift detection logic
		status, _ := obj[objects.FieldKeyStatus].(string)
		updatedAt, _ := obj[objects.FieldKeyUpdatedAt].(time.Time)

		if status == "planned" && time.Since(updatedAt) > 24*time.Hour {
			drifts = append(drifts, DriftReport{
				BacklogItemID: obj[objects.FieldKeyID].(string),
				Severity:      "medium",
				Message:       "Backlog item planned but not updated in 24h",
			})
		}
	}
	return drifts, nil
}
