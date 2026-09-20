package systemcheck

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

type mockStorageForTelemetry struct {
	storagepkg.ObjectStorageProvider
	objectsByKind map[string][]map[string]any
}

func (m *mockStorageForTelemetry) List(
	_ context.Context,
	_ *pkgctx.SecurityContext,
	_ *pkgctx.StorageContext,
	filter storagepkg.ListFilter,
) (*storagepkg.QueryResult, error) {
	items := m.objectsByKind[filter.Kind]
	return &storagepkg.QueryResult{Objects: items}, nil
}

func TestTPMFeedbackTelemetry(t *testing.T) {
	t.Parallel()

	mock := &mockStorageForTelemetry{
		objectsByKind: map[string][]map[string]any{
			objects.KindCriteria: {
				{
					objects.FieldKeyID:     "CRIT-001",
					objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
				},
				{
					objects.FieldKeyID:     "CRIT-002",
					objects.FieldKeyStatus: objects.ObjectStatusComplete,
				},
			},
			objects.KindBacklogItem: {
				{
					objects.FieldKeyID:          "BLI-001",
					objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
					objects.FieldKeyDescription: "", // hygiene anomaly
				},
				{
					objects.FieldKeyID:          "BLI-002",
					objects.FieldKeyStatus:      objects.ObjectStatusComplete,
					objects.FieldKeyDescription: "Complete description here",
				},
			},
		},
	}

	ctx := context.Background()
	telemetry, err := CollectTPMFeedbackTelemetry(ctx, mock)
	if err != nil {
		t.Fatalf("CollectTPMFeedbackTelemetry failed: %v", err)
	}

	if telemetry.UnverifiedCriteria != 1 {
		t.Errorf("expected 1 unverified criteria, got %d", telemetry.UnverifiedCriteria)
	}
	if telemetry.StalledBacklogItems != 1 {
		t.Errorf("expected 1 stalled backlog item, got %d", telemetry.StalledBacklogItems)
	}
	if telemetry.HygieneAnomalies != 1 {
		t.Errorf("expected 1 hygiene anomaly, got %d", telemetry.HygieneAnomalies)
	}

	if len(telemetry.RefinementActions) != 3 {
		t.Fatalf("expected 3 refinement actions, got %d: %#v", len(telemetry.RefinementActions), telemetry.RefinementActions)
	}

	foundCritAction := false
	foundBliAction := false
	foundHygieneAction := false

	for _, act := range telemetry.RefinementActions {
		if strings.Contains(act, "tpm-refine-unverified-criteria") {
			foundCritAction = true
		}
		if strings.Contains(act, "tpm-sweep-in-progress-blis") {
			foundBliAction = true
		}
		if strings.Contains(act, "tpm-remediate-hygiene-anomalies") {
			foundHygieneAction = true
		}
	}

	if !foundCritAction {
		t.Errorf("expected unverified criteria action in refinement actions")
	}
	if !foundBliAction {
		t.Errorf("expected stalled BLI action in refinement actions")
	}
	if !foundHygieneAction {
		t.Errorf("expected hygiene anomaly action in refinement actions")
	}

	if len(telemetry.RankedActionClusters) != 3 {
		t.Errorf("expected 3 action clusters, got %d", len(telemetry.RankedActionClusters))
	}
}
